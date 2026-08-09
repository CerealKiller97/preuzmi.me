package mailbox

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// netTimeout bounds every IMAP network operation. Mailboxes are fetched from a
// background refresh, so a stuck connection must not hang the whole run.
const netTimeout = 30 * time.Second

// maxFetch caps how many matching messages are pulled in one search. eUpravnik
// only needs the newest bill, so a small window keeps memory bounded even if the
// filter is loose.
const maxFetch = 20

// IMAPReader reads a mailbox over IMAP-over-TLS. It holds no connection: each
// Search dials, does its work, and logs out, which keeps it safe to reuse across
// refreshes without minding a dropped socket.
type IMAPReader struct {
	addr     string
	username string
	password string
	mailbox  string
}

var _ Reader = (*IMAPReader)(nil)

// NewIMAP builds a reader for the given mailbox config and credentials. The
// address is resolved up front so a misconfigured provider fails at construction
// rather than on the first refresh.
func NewIMAP(cfg Config, username, password string) (*IMAPReader, error) {
	addr, err := cfg.Address()
	if err != nil {
		return nil, err
	}

	return &IMAPReader{
		addr:     addr,
		username: username,
		password: password,
		mailbox:  cfg.MailboxOrDefault(),
	}, nil
}

// Search connects, runs a server-side SEARCH for the criteria, fetches the
// matching messages (newest first) and returns them with attachments decoded.
func (r *IMAPReader) Search(ctx context.Context, c Criteria) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cl, err := client.DialTLS(r.addr, nil)
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", r.addr, err)
	}
	cl.Timeout = netTimeout
	// Best-effort logout; the connection is torn down regardless.
	defer cl.Logout() //nolint:errcheck

	if err := cl.Login(r.username, r.password); err != nil {
		return nil, fmt.Errorf("imap login: %w", err)
	}

	if _, err := cl.Select(r.mailbox, true); err != nil {
		return nil, fmt.Errorf("imap select %q: %w", r.mailbox, err)
	}

	seqNums, err := cl.Search(searchCriteria(c))
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}

	if len(seqNums) == 0 {
		return nil, nil
	}

	// Search returns sequence numbers ascending; the highest ones are the most
	// recent messages, so keep the tail and fetch newest first.
	if len(seqNums) > maxFetch {
		seqNums = seqNums[len(seqNums)-maxFetch:]
	}

	return r.fetch(ctx, cl, seqNums)
}

// fetch pulls the given messages in full and decodes each into a Message,
// ordered newest first.
func (r *IMAPReader) fetch(ctx context.Context, cl *client.Client, seqNums []uint32) ([]Message, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(seqNums...)

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}

	ch := make(chan *imap.Message, len(seqNums))
	done := make(chan error, 1)
	go func() {
		done <- cl.Fetch(seqset, items, ch)
	}()

	out := make([]Message, 0, len(seqNums))
	for msg := range ch {
		parsed, err := parseMessage(msg, section)
		if err != nil {
			// Skip a message we cannot parse rather than failing the whole run:
			// one malformed mail must not hide the others.
			continue
		}
		out = append(out, parsed)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fetch delivers messages in ascending sequence order; reverse so the caller
	// gets newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out, nil
}

// searchCriteria translates the package Criteria into the IMAP search form.
func searchCriteria(c Criteria) *imap.SearchCriteria {
	criteria := imap.NewSearchCriteria()
	if c.From != "" {
		criteria.Header.Add("From", c.From)
	}
	if c.Subject != "" {
		criteria.Header.Add("Subject", c.Subject)
	}
	if !c.Since.IsZero() {
		criteria.Since = c.Since
	}

	return criteria
}

// parseMessage turns a fetched IMAP message into a Message, reading its
// attachments out of the raw body.
func parseMessage(msg *imap.Message, section *imap.BodySectionName) (Message, error) {
	if msg == nil {
		return Message{}, fmt.Errorf("nil message")
	}

	body := msg.GetBody(section)
	if body == nil {
		return Message{}, fmt.Errorf("message has no body section")
	}

	out := Message{}
	if env := msg.Envelope; env != nil {
		out.Subject = env.Subject
		out.Date = env.Date
		if len(env.From) > 0 {
			out.From = env.From[0].Address()
		}
	}
	// The envelope date can be missing or unparseable; fall back to the server's
	// internal date, which is what the Since filter matched on.
	if out.Date.IsZero() {
		out.Date = msg.InternalDate
	}

	attachments, err := readAttachments(body)
	if err != nil {
		return Message{}, err
	}
	out.Attachments = attachments

	return out, nil
}

// readAttachments walks the MIME parts of a raw message and returns every
// attachment part, decoded. Inline parts and the message body are ignored.
func readAttachments(r io.Reader) ([]Attachment, error) {
	mr, err := mail.CreateReader(r)
	if err != nil {
		// A message that is not MIME multipart simply has no attachments.
		if message.IsUnknownCharset(err) {
			return nil, nil
		}
		return nil, err
	}
	defer mr.Close() //nolint:errcheck

	var attachments []Attachment
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Tolerate an unknown charset on a single part and keep reading the
			// rest of the message.
			if message.IsUnknownCharset(err) {
				continue
			}
			return nil, err
		}

		h, ok := p.Header.(*mail.AttachmentHeader)
		if !ok {
			continue
		}

		filename, _ := h.Filename()
		contentType, _, _ := h.ContentType()

		data, err := io.ReadAll(p.Body)
		if err != nil {
			return nil, err
		}

		attachments = append(attachments, Attachment{
			Filename:    filename,
			ContentType: contentType,
			Data:        data,
		})
	}

	return attachments, nil
}
