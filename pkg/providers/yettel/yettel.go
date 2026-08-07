// Package yettel downloads the latest Yettel invoice. Yettel emails the invoice
// as a PDF attachment, so — like eUpravnik — this provider reads the mailbox
// over IMAP (via the mailbox package), saves the newest attachment, and parses
// the PDF for the amount and billing period.
package yettel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/ipsqr"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/mailbox"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
)

// senderFilter optionally narrows the mailbox search to Yettel's invoice address
// (a substring IMAP FROM match). It is empty by default: the intended setup is a
// dedicated Gmail label (configured as the provider's mailbox) that already
// contains only Yettel's mail, so no sender filter is needed. Set it to Yettel's
// address if you search a shared folder like INBOX instead.
var senderFilter = ""

// searchTimeout bounds the whole mailbox round-trip.
const searchTimeout = 45 * time.Second

var _ provider.Interface = (*Service)(nil)

// Service downloads the Yettel invoice from a mailbox.
type Service struct {
	logger   zerolog.Logger
	storage  storage.Interface
	receipts *receipts.Repository
	reader   mailbox.Reader
	// name is the account key: "yettel" for the primary account, or
	// "yettel-<slug>" for an extra one (a second mailbox/login). It is the
	// receipt's base filename and its provider column in the receipts index.
	name string
}

// New builds the provider. name is the account key, reader is the mailbox to
// search (injected so tests can supply a fake inbox); storage and receiptsStore
// are shared with the other providers.
func New(
	name string,
	reader mailbox.Reader,
	logger zerolog.Logger,
	storage storage.Interface,
	receiptsStore *receipts.Repository,
) *Service {
	return &Service{
		name:     name,
		reader:   reader,
		logger:   logger,
		storage:  storage,
		receipts: receiptsStore,
	}
}

func (s *Service) DownloadReceipt() error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	messages, err := s.reader.Search(ctx, mailbox.Criteria{
		From:  senderFilter,
		Since: searchSince(time.Now()),
	})
	if err != nil {
		s.logger.Err(err).Msg("Error while searching the mailbox")
		return err
	}

	s.logger.Info().Int("messages", len(messages)).Msg("Searched mailbox for Yettel invoice")

	msg, attachment, ok := latestPDF(messages)
	if !ok {
		// No invoice email waiting yet — the mailbox is empty or the bill has not
		// arrived. That is a normal "nothing to fetch", not a failure: the sentinel
		// lets the refresh runner mark this provider as "no email" rather than an
		// error, so the UI can say so and a scheduled run stays quiet.
		s.logger.Info().Msg("No Yettel invoice email found yet; nothing to download")
		return provider.ErrNoReceipt
	}

	// Parse the PDF up front so the billing period it names decides the folder,
	// rather than whenever the email happened to arrive. A parse failure is not
	// fatal: the PDF is still saved under the email-date period.
	in, parseErr := parseInvoice(attachment.Data)
	if parseErr != nil {
		s.logger.Err(parseErr).Msg("Could not parse Yettel PDF; filing under email date, price left unset")
	}

	period := periodFromDate(msg.Date)
	if parseErr == nil && in.hasPeriod {
		period = in.periodString()
	}

	key := fmt.Sprintf("%s/%s.pdf", period, s.name)
	if err := s.storage.Save(ctx, key, attachment.Data); err != nil {
		s.logger.Err(err).Str("key", key).Msg("Error saving Yettel receipt")
		return err
	}

	s.logger.Info().
		Str("key", key).
		Str("attachment", attachment.Filename).
		Time("emailDate", msg.Date).
		Msg("Successfully downloaded receipt")

	// Record price and paid status. Best-effort: the PDF is already saved, so a
	// parse or database hiccup must not fail the download.
	if s.receipts != nil && parseErr == nil {
		s.recordInvoice(ctx, in, period, attachment.Data)
	}

	return nil
}

// recordInvoice stores the parsed price and paid status against the receipts
// index. Every failure is logged and swallowed: the receipt is already on disk.
//
// Yettel's "Prethodno stanje" is the balance carried in from before this
// invoice, so a fresh bill never proves itself paid — it proves the previous
// month is. The status model mirrors eUpravnik:
//
//   - the just-downloaded (current) receipt starts as unpaid; it is the newly
//     issued bill, due later, and the next month's invoice will confirm it.
//   - when this invoice carries no previous balance (Prethodno stanje == 0), the
//     previous month's receipt is settled, so it flips to paid.
func (s *Service) recordInvoice(ctx context.Context, in invoice, period string, pdf []byte) {
	// The IPS QR carries the exact amount a banking app charges, so it is the
	// source of truth for the price. Fall back to the PDF's "UKUPNO" total only
	// when the bill has no readable QR.
	if amount, ok := ipsqr.AmountFromPDF(pdf); ok {
		if err := s.receipts.SetPrice(ctx, s.name, period, amount); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record Yettel receipt price")
		}
	} else if in.hasTotal {
		if err := s.receipts.SetPrice(ctx, s.name, period, in.total); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record Yettel receipt price")
		}
	}

	// Never leave the status empty: the current invoice is unpaid until a later
	// invoice confirms it.
	if err := s.receipts.SetStatus(ctx, s.name, period, receipts.StatusUnpaid); err != nil {
		s.logger.Err(err).Str("period", period).Msg("Failed to record Yettel receipt status")
	}

	// A zero carried-over balance settles the previous month. Requires the parsed
	// period so we know which month "previous" is.
	if in.hasPrevBalance && in.hasPeriod && in.previousSettled() {
		prev := in.previousPeriodString()
		if err := s.receipts.SetStatus(ctx, s.name, prev, receipts.StatusPaid); err != nil {
			s.logger.Err(err).Str("period", prev).Msg("Failed to mark previous Yettel receipt paid")
		}
	}
}

// searchSince returns the start of the previous month relative to now. That
// window is wide enough to catch the current bill regardless of when in the
// month it was sent, while keeping the fetch small.
func searchSince(now time.Time) time.Time {
	year, month := now.Year(), now.Month()

	month--
	if month < 1 {
		month = 12
		year--
	}

	return time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
}

// latestPDF picks the newest message (by date) that carries a PDF attachment and
// returns that message, its first PDF attachment, and whether one was found.
func latestPDF(messages []mailbox.Message) (mailbox.Message, mailbox.Attachment, bool) {
	var (
		best    mailbox.Message
		bestAtt mailbox.Attachment
		bestSet bool
	)

	for _, m := range messages {
		att, ok := firstPDF(m.Attachments)
		if !ok {
			continue
		}

		if !bestSet || m.Date.After(best.Date) {
			best, bestAtt, bestSet = m, att, true
		}
	}

	return best, bestAtt, bestSet
}

// firstPDF returns the first PDF attachment in the slice, matching either the
// content type or a ".pdf" filename (some senders mislabel the content type).
func firstPDF(attachments []mailbox.Attachment) (mailbox.Attachment, bool) {
	for _, a := range attachments {
		if len(a.Data) == 0 {
			continue
		}
		if strings.EqualFold(a.ContentType, "application/pdf") ||
			strings.HasSuffix(strings.ToLower(a.Filename), ".pdf") {
			return a, true
		}
	}

	return mailbox.Attachment{}, false
}

// periodFromDate returns the "MM-YYYY" folder for the email's date, falling back
// to the previous-month heuristic when the date is missing.
func periodFromDate(t time.Time) string {
	if t.IsZero() {
		return utils.PreviousMonthFolder()
	}

	return fmt.Sprintf("%02d-%d", int(t.Month()), t.Year())
}
