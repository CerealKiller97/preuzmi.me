package notify_test

import (
	"strings"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// senderFunc adapts a function to the notify.Sender interface for tests.
type senderFunc func(subject, body string) error

func (f senderFunc) Send(subject, body string) error { return f(subject, body) }

func TestMessagesPerReceipt(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	results := []refresh.Result{
		{Provider: "a1", OK: true, DurationMS: 1200, Period: "05-2026", Price: 1819.46},
		{Provider: "mts", OK: false, Error: "login failed"},
	}

	msgs := notify.Messages(cfg, results)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, "A1")
	assert.Contains(t, msgs[0].Body, "A1")
	assert.Contains(t, msgs[0].Body, "05/2026", "body should name the billing month/year")
	assert.Contains(t, msgs[0].Body, "1.819,46 RSD", "body should include the price")
	assert.NotContains(t, msgs[0].Body, "s)", "duration must be removed")
}

func TestMessagesPerReceiptWithoutEnrichment(t *testing.T) {
	// When the run recorded no period/price, the message still sends cleanly
	// without a month or amount and without the old duration suffix.
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	msgs := notify.Messages(cfg, []refresh.Result{{Provider: "eps", OK: true, DurationMS: 3400}})

	require.Len(t, msgs, 1)
	assert.Equal(t, "Račun za EPS je uspešno preuzet.\n", msgs[0].Body)
}

func TestMessagesAllDoneRequiresEveryProvider(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModeAllDone}

	assert.Empty(t, notify.Messages(cfg, []refresh.Result{
		{Provider: "mts", OK: true},
		{Provider: "a1", OK: false},
	}))

	msgs := notify.Messages(cfg, []refresh.Result{
		{Provider: "mts", OK: true},
		{Provider: "a1", OK: true},
	})
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, "svi računi")
	assert.Contains(t, msgs[0].Body, "MTS")
	assert.Contains(t, msgs[0].Body, "A1")
}

func TestHandleVerifiedSendsMessages(t *testing.T) {
	var sent []struct{ subject, body string }
	sender := senderFunc(func(subject, body string) error {
		sent = append(sent, struct{ subject, body string }{subject, body})
		return nil
	})

	// Mode off, but paid-confirmation on: the message still fires.
	svc := notify.New(config.Notifications{Mode: config.NotifyModeOff, Driver: config.NotifyDriverTelegram, PaidConfirmation: true}, sender, zerolog.Nop())
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "mts", Period: "05-2026", Price: 1819.46}})

	require.Len(t, sent, 1)
	assert.True(t, strings.HasPrefix(sent[0].subject, "✅ "), "telegram subject should lead with the paid emoji")
	assert.Contains(t, sent[0].subject, "MTS")
	assert.Contains(t, sent[0].subject, "potvrđen")
	assert.Contains(t, sent[0].body, "05/2026")
	assert.Contains(t, sent[0].body, "1.819,46 RSD")
}

func TestSMTPSubjectsHaveEmoji(t *testing.T) {
	var sent []struct{ subject, body string }
	sender := senderFunc(func(subject, body string) error {
		sent = append(sent, struct{ subject, body string }{subject, body})
		return nil
	})

	// SMTP driver: subjects carry the same emoji prefix as Telegram.
	svc := notify.New(config.Notifications{Mode: config.NotifyModeOff, Driver: config.NotifyDriverSMTP, PaidConfirmation: true}, sender, zerolog.Nop())
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "mts", Period: "05-2026", Price: 1819.46}})

	require.Len(t, sent, 1)
	assert.True(t, strings.HasPrefix(sent[0].subject, "✅ "), "email subject should lead with the paid emoji")
	assert.Contains(t, sent[0].subject, "preuzmi.me:")
}

func TestHandleVerifiedSilentWhenPaidConfirmationOff(t *testing.T) {
	var count int
	sender := senderFunc(func(string, string) error { count++; return nil })
	// Download mode active, but paid-confirmation not opted in => silent.
	svc := notify.New(config.Notifications{Mode: config.NotifyModePerReceipt, Driver: config.NotifyDriverTelegram}, sender, zerolog.Nop())
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "eps", Period: "06-2026"}})
	assert.Equal(t, 0, count)
}

func TestMessagesOffSendsNothing(t *testing.T) {
	assert.Empty(t, notify.Messages(config.Notifications{Mode: config.NotifyModeOff}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
	assert.Empty(t, notify.Messages(config.Notifications{}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
}
