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
}

// New builds a Service from config. When mode is off, sender may still be set
// so Test works against the configured driver.
func New(cfg config.Notifications, sender Sender, log zerolog.Logger) *Service {
	return &Service{cfg: cfg, sender: sender, log: log}
}

// NewFromConfig picks the driver implementation for the configured backend.
func NewFromConfig(cfg *config.Config, log zerolog.Logger) (*Service, error) {
	n := cfg.Notifications

	sender, err := buildSender(n)
	if err != nil {
		return nil, err
	}

	if n.NotifyEnabled() && sender == nil {
		return nil, fmt.Errorf("notifications are enabled but driver %q is not configured", n.Driver)
	}

	return New(n, sender, log), nil
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

// Test sends a fixed probe message so the user can verify the driver without
// running a full receipt download.
func (s *Service) Test() error {
	if !s.CanTest() {
		return ErrNotConfigured
	}

	return s.sender.Send(
		"preuzmi.me: test",
		"Ovo je test obaveštenje. Ako ga vidite, drajver je ispravno podešen.\n",
	)
}

// HandleResults sends the configured notifications for a finished refresh.
// Failures are logged and never bubbled up: a delivery outage must not fail
// the download itself.
func (s *Service) HandleResults(results []refresh.Result) {
	if !s.Enabled() {
		return
	}

	for _, msg := range Messages(s.cfg, results) {
		if err := s.sender.Send(msg.Subject, msg.Body); err != nil {
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

// Message is one outbound notification.
type Message struct {
	Subject string
	Body    string
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
			if !r.OK {
				continue
			}
			name := strings.ToUpper(r.Provider)
			out = append(out, Message{
				Subject: fmt.Sprintf("preuzmi.me: račun %s preuzet", name),
				Body: fmt.Sprintf(
					"Račun za %s je uspešno preuzet (%.1fs).\n",
					name,
					float64(r.DurationMS)/1000,
				),
			})
		}
		return out

	case config.NotifyModeAllDone:
		if !allSucceeded(results) {
			return nil
		}
		names := make([]string, 0, len(results))
		for _, r := range results {
			names = append(names, strings.ToUpper(r.Provider))
		}
		return []Message{{
			Subject: "preuzmi.me: svi računi preuzeti",
			Body: fmt.Sprintf(
				"Svi konfigurisani provajderi su uspešno preuzeti: %s.\n",
				strings.Join(names, ", "),
			),
		}}

	default:
		return nil
	}
}

func allSucceeded(results []refresh.Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}

	return true
}
