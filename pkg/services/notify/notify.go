// Package notify delivers messages when receipt downloads finish.
//
// notifications.mode picks exactly one behaviour:
//   - off: nothing is sent automatically
//   - per_receipt: one message per successfully downloaded provider
//   - all_done: one message when every provider in the run succeeded
//
// The driver (smtp or telegram) is selected once and shared by every mode.
package notify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/script"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/rs/zerolog"
)

// ErrNotConfigured is returned by Test when the selected driver has no
// usable credentials yet.
var ErrNotConfigured = errors.New("notification driver is not configured")

// Sender delivers a single message. Implementations must be safe to call from
// a background goroutine (the refresh worker).
type Sender interface {
	Send(subject, body string) error
}

// Service decides which messages to send after a refresh finishes.
type Service struct {
	log    zerolog.Logger
	sender Sender
	cfg    config.Notifications
	lang   string
}

// New builds a Service from config. When mode is off, sender may still be set
// so Test works against the configured driver.
func New(cfg config.Notifications, sender Sender, log zerolog.Logger) *Service {
	return NewWithLang(cfg, sender, log, config.LangLatin)
}

// NewWithLang is like New but applies the given Serbian script to message text.
func NewWithLang(cfg config.Notifications, sender Sender, log zerolog.Logger, lang string) *Service {
	return &Service{
		cfg:    cfg,
		sender: sender,
		log:    log,
		lang:   lang,
	}
}

// NewFromConfig picks the driver implementation for the configured backend.
func NewFromConfig(cfg *config.Config, log zerolog.Logger) (*Service, error) {
	n := cfg.Notifications

	sender, err := buildSender(n)
	if err != nil {
		return nil, err
	}

	if n.DeliveryEnabled() && sender == nil {
		return nil, fmt.Errorf("notifications are enabled but driver %q is not configured", n.Driver)
	}

	return NewWithLang(n, sender, log, cfg.Lang), nil
}

func buildSender(n config.Notifications) (Sender, error) {
	switch n.Driver {
	case config.NotifyDriverTelegram:
		if n.Telegram.BotToken == "" || n.Telegram.ChatID == "" {
			return nil, nil
		}
		return NewTelegram(n.Telegram)
	case config.NotifyDriverSMTP, "":
		if n.SMTP.Host == "" || n.SMTP.From == "" || n.SMTP.To == "" {
			return nil, nil
		}
		return NewSMTP(n.SMTP)
	default:
		return nil, fmt.Errorf("unsupported notifications driver %q", n.Driver)
	}
}

// Enabled reports whether automatic notifications will fire after a refresh.
func (s *Service) Enabled() bool {
	return s != nil && s.sender != nil && s.cfg.NotifyEnabled()
}

// CanTest reports whether a manual test message can be sent.
func (s *Service) CanTest() bool {
	return s != nil && s.sender != nil
}

// PaidConfirmationEnabled reports whether paid-confirmation notifications will
// fire. It is independent of the download-notification Mode.
func (s *Service) PaidConfirmationEnabled() bool {
	return s != nil && s.sender != nil && s.cfg.PaidConfirmation
}

// Test sends a fixed probe message so the user can verify the driver without
// running a full receipt download.
func (s *Service) Test() error {
	if !s.CanTest() {
		return ErrNotConfigured
	}

	return s.send(Message{
		Emoji:   emojiTest,
		Subject: "preuzmi.me: test",
		Body:    "Ovo je test obaveštenje. Ako ga vidite, drajver je ispravno podešen.\n",
	})
}

// send delivers one message through the configured driver. The message's emoji
// is prepended to the subject to make notifications scannable, for both Telegram
// chats and email inboxes. Subject and body are transliterated when lang is
// cyrillic so Telegram and SMTP stay in sync with the UI script.
func (s *Service) send(m Message) error {
	subject := script.ApplyReplacing(s.lang, m.Subject, m.Repl)
	if m.Emoji != "" {
		subject = m.Emoji + " " + subject
	}

	return s.sender.Send(subject, script.ApplyReplacing(s.lang, m.Body, m.Repl))
}

// HandleResults sends the configured notifications for a finished refresh.
// Failures are logged and never bubbled up: a delivery outage must not fail
// the download itself.
func (s *Service) HandleResults(results []refresh.Result) {
	if !s.Enabled() {
		return
	}

	for _, msg := range Messages(s.cfg, results) {
		if err := s.send(msg); err != nil {
			s.log.Err(err).
				Str("subject", msg.Subject).
				Msg("Failed to send notification")
		} else {
			s.log.Info().
				Str("subject", msg.Subject).
				Msg("Notification sent")
		}
	}
}

// Emojis prefixed to notification subjects, one per message kind, so a glance at
// the chat or inbox tells the kind apart (see Service.send).
const (
	emojiReceipt = "🧾" // a receipt was downloaded
	emojiAllDone = "✅" // every provider in the run succeeded
	emojiPaid    = "✅" // a receipt was confirmed paid
	emojiTest    = "🔔" // manual test probe
)

// providerNames overrides the Latin display name for provider keys whose plain
// uppercased form would read wrong: the Serbian-word providers need an explicit
// "E-" spelling so they render as "E-SANDUČE" / "E-UPRAVNIK". Providers not
// listed here use the uppercased key.
var providerNames = map[string]string{
	"esanduce":  "E-SANDUČE",
	"eupravnik": "E-UPRAVNIK",
}

// providerCyrillic pins the Cyrillic form for providers whose name does not
// simply transliterate: a foreign brand kept in Latin (A1 → A1) or a custom
// spelling (Yettel → ЈЕТЕЛ, since transliterating "YETTEL" mangles the Y). Every
// other provider transliterates normally, including Serbian acronyms (MTS → МТС,
// EPS → ЕПС).
var providerCyrillic = map[string]string{
	"a1":     "A1",
	"yettel": "ЈЕТЕЛ",
}

// displayName returns the Latin display name for a provider key: an explicit
// override, or the uppercased key otherwise. send transliterates it, except for
// the fixed Cyrillic forms in providerCyrillic (see addRepl / Message.Repl).
func displayName(provider string) string {
	if name, ok := providerNames[strings.ToLower(provider)]; ok {
		return name
	}
	return strings.ToUpper(provider)
}

// addRepl records the Latin→Cyrillic substitution for a provider whose Cyrillic
// name is fixed (providerCyrillic). Providers that transliterate normally add
// nothing and leave repl untouched.
func addRepl(repl map[string]string, provider, name string) map[string]string {
	c, ok := providerCyrillic[strings.ToLower(provider)]
	if !ok {
		return repl
	}
	if repl == nil {
		repl = make(map[string]string, 1)
	}
	repl[name] = c
	return repl
}

// Message is one outbound notification. Emoji, when set, is prepended to the
// subject for Telegram deliveries only.
type Message struct {
	Emoji   string
	Subject string
	Body    string
	// Repl maps a Latin provider name to its fixed Cyrillic form, applied when
	// the message is transliterated (see providerCyrillic).
	Repl map[string]string
}

// Messages builds the outbound set for the given mode and results without
// sending anything. Extracted so the decision logic can be unit-tested.
func Messages(cfg config.Notifications, results []refresh.Result) []Message {
	if len(results) == 0 {
		return nil
	}

	switch cfg.Mode {
	case config.NotifyModePerReceipt:
		out := make([]Message, 0)
		for _, r := range results {
			// Only announce a receipt downloaded for the first time this run, so a
			// daily re-download of an already-saved bill stays silent.
			if !r.OK || !r.New {
				continue
			}
			name := displayName(r.Provider)
			out = append(out, Message{
				Emoji:   emojiReceipt,
				Subject: fmt.Sprintf("preuzmi.me: račun %s preuzet", name),
				Body:    perReceiptBody(name, r),
				Repl:    addRepl(nil, r.Provider, name),
			})
		}
		return out

	case config.NotifyModeAllDone:
		// Every provider succeeded, and at least one receipt was new — so the
		// summary fires when a run completes with fresh downloads, not on every
		// daily re-run once everything is already saved.
		if !allSucceeded(results) || !anyNew(results) {
			return nil
		}
		names := make([]string, 0, len(results))
		var repl map[string]string
		for _, r := range results {
			name := displayName(r.Provider)
			names = append(names, name)
			repl = addRepl(repl, r.Provider, name)
		}
		return []Message{{
			Emoji:   emojiAllDone,
			Subject: "preuzmi.me: svi računi preuzeti",
			Body: fmt.Sprintf(
				"Svi konfigurisani provajderi su uspešno preuzeti: %s.\n",
				strings.Join(names, ", "),
			),
			Repl: repl,
		}}

	default:
		return nil
	}
}

// VerifiedReceipt describes a receipt whose provider just confirmed payment.
type VerifiedReceipt struct {
	Provider string
	Period   string
	Price    float64
}

// HandleVerified sends a notification for each receipt the provider newly
// confirmed as paid. It fires whenever paid-confirmation is enabled, regardless
// of the download-notification Mode. Failures are logged and never bubbled up.
func (s *Service) HandleVerified(items []VerifiedReceipt) {
	if !s.PaidConfirmationEnabled() {
		return
	}

	for _, it := range items {
		msg := verifiedMessage(it)
		if err := s.send(msg); err != nil {
			s.log.Err(err).
				Str("subject", msg.Subject).
				Msg("Failed to send verified notification")
		} else {
			s.log.Info().Str("subject", msg.Subject).Msg("Verified notification sent")
		}
	}
}

// verifiedMessage builds the message for a payment-confirmed receipt.
func verifiedMessage(it VerifiedReceipt) Message {
	name := displayName(it.Provider)

	var b strings.Builder
	fmt.Fprintf(&b, "Račun za %s za %s je potvrđen kao plaćen.", name, formatPeriod(it.Period))
	if it.Price > 0 {
		fmt.Fprintf(&b, " Iznos: %s.", formatPrice(it.Price))
	}
	b.WriteString("\n")

	return Message{
		Emoji:   emojiPaid,
		Subject: fmt.Sprintf("preuzmi.me: račun %s potvrđen", name),
		Body:    b.String(),
		Repl:    addRepl(nil, it.Provider, name),
	}
}

// perReceiptBody builds the per-receipt notification text, naming the billing
// month/year and amount when the run recorded them.
func perReceiptBody(name string, r refresh.Result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Račun za %s", name)
	if r.Period != "" {
		fmt.Fprintf(&b, " za %s", formatPeriod(r.Period))
	}
	b.WriteString(" je uspešno preuzet.")

	if r.Price > 0 {
		fmt.Fprintf(&b, " Iznos: %s.", formatPrice(r.Price))
	}

	b.WriteString("\n")

	return b.String()
}

// formatPeriod turns the stored "MM-YYYY" period into "MM/YYYY" for display,
// leaving anything unexpected untouched.
func formatPeriod(period string) string {
	if month, year, ok := strings.Cut(period, "-"); ok {
		return month + "/" + year
	}

	return period
}

// formatPrice renders an amount in Serbian style (comma decimal separator) with
// the RSD suffix, e.g. 1819.46 -> "1.819,46 RSD".
func formatPrice(price float64) string {
	whole := int64(price)
	cents := int64((price-float64(whole))*100 + 0.5)

	// Group the integer part with "." thousands separators.
	digits := fmt.Sprintf("%d", whole)
	var grouped strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(d)
	}

	return fmt.Sprintf("%s,%02d RSD", grouped.String(), cents)
}

func allSucceeded(results []refresh.Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}

	return true
}

// anyNew reports whether the run downloaded at least one receipt for the first
// time, so a summary is not sent when nothing actually changed.
func anyNew(results []refresh.Result) bool {
	for _, r := range results {
		if r.OK && r.New {
			return true
		}
	}

	return false
}
