// Package mailbox reads email over IMAP so providers that only deliver their
// documents by email (e.g. eUpravnik) can fetch them.
//
// It is provider-agnostic on purpose: Gmail, Outlook, Yahoo and iCloud all
// speak IMAP, so a single implementation covers every mailbox once the server
// host is known. Callers depend on the Reader interface, which keeps the IMAP
// details out of the services and lets tests supply a fake inbox.
package mailbox

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Provider presets name the well-known IMAP servers so users pick a provider
// instead of hunting for host names. "custom" means the host/port are given
// explicitly in config.
const (
	ProviderGmail   = "gmail"
	ProviderOutlook = "outlook"
	ProviderYahoo   = "yahoo"
	ProviderICloud  = "icloud"
	ProviderCustom  = "custom"
)

// DefaultMailbox is the folder searched when none is configured. Callers can
// point Config.Mailbox at a folder (or a Gmail label) to narrow the search.
const DefaultMailbox = "INBOX"

// server is a resolved IMAP endpoint.
type server struct {
	host string
	port int
}

// presets maps a provider name onto its IMAP-over-TLS endpoint (port 993).
var presets = map[string]server{
	ProviderGmail:   {"imap.gmail.com", 993},
	ProviderOutlook: {"outlook.office365.com", 993},
	ProviderYahoo:   {"imap.mail.yahoo.com", 993},
	ProviderICloud:  {"imap.mail.me.com", 993},
}

// Config selects which IMAP server to talk to. It carries no secrets: the
// mailbox username and password are the provider's own credentials, passed to
// Dial separately.
type Config struct {
	// Provider is one of the Provider* constants. An empty value defaults to
	// "custom", requiring Host (and optionally Port).
	Provider string `json:"provider"`
	// Host and Port override the preset; they are required when Provider is
	// "custom" and ignored otherwise unless the preset is unknown.
	Host string `json:"host"`
	Port int    `json:"port"`
	// Mailbox is the folder to search, defaulting to INBOX. On Gmail every
	// label is exposed as an IMAP folder, so setting this to a label name
	// scopes the server-side search to just that label's messages instead of
	// the whole inbox — a meaningful speed-up on large mailboxes. Nested
	// labels use "/" as the separator, e.g. "Racuni/eUpravnik".
	Mailbox string `json:"mailbox"`
}

// Address returns the "host:port" to dial for the configured provider, applying
// presets and defaults. It errors when a custom provider omits the host.
func (c Config) Address() (string, error) {
	provider := strings.ToLower(strings.TrimSpace(c.Provider))
	if provider == "" {
		provider = ProviderCustom
	}

	if provider != ProviderCustom {
		if s, ok := presets[provider]; ok {
			host, port := s.host, s.port
			// Explicit overrides still win over the preset, so an unusual port
			// or a proxy host can be forced.
			if c.Host != "" {
				host = c.Host
			}
			if c.Port != 0 {
				port = c.Port
			}

			return fmt.Sprintf("%s:%d", host, port), nil
		}

		return "", fmt.Errorf("unknown email provider %q", c.Provider)
	}

	if c.Host == "" {
		return "", fmt.Errorf("email provider is %q but email host is empty", ProviderCustom)
	}

	port := c.Port
	if port == 0 {
		port = 993
	}

	return fmt.Sprintf("%s:%d", c.Host, port), nil
}

// MailboxOrDefault returns the configured folder, or INBOX when unset.
func (c Config) MailboxOrDefault() string {
	if strings.TrimSpace(c.Mailbox) == "" {
		return DefaultMailbox
	}

	return c.Mailbox
}

// Attachment is one decoded attachment of a message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is a single email with its attachments already decoded.
type Message struct {
	From        string
	Subject     string
	Date        time.Time
	Attachments []Attachment
}

// Criteria narrows the mailbox search. Zero-value fields are not applied, so an
// empty Criteria matches every message in the folder.
type Criteria struct {
	// From matches the sender's address (IMAP FROM, a substring match).
	From string
	// Since matches messages whose internal date is on or after it.
	Since time.Time
	// Subject matches the subject line (IMAP SUBJECT, a substring match).
	Subject string
}

// Reader reads messages from a mailbox. Implementations connect lazily inside
// Search so a Reader is cheap to construct and safe to keep on a service.
type Reader interface {
	Search(ctx context.Context, c Criteria) ([]Message, error)
}
