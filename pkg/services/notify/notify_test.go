package notify_test

import (
	"strings"
	"testing"
	"time"

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
		{Provider: "a1", OK: true, New: true, DurationMS: 1200, Period: "05-2026", Price: 1819.46},
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

// A successful re-download that is not new must not produce a message, so a
// daily cron does not re-notify about a receipt already on record.
func TestMessagesPerReceiptSkipsAlreadyDownloaded(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	msgs := notify.Messages(cfg, []refresh.Result{
		{Provider: "a1", OK: true, New: false, Period: "05-2026"},
		{Provider: "mts", OK: true, New: true, Period: "05-2026"},
	})

	require.Len(t, msgs, 1, "only the newly downloaded receipt should notify")
	assert.Contains(t, msgs[0].Subject, "MTS")
}

func TestMessagesPerReceiptWithoutEnrichment(t *testing.T) {
	// When the run recorded no period/price, the message still sends cleanly
	// without a month or amount and without the old duration suffix.
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	msgs := notify.Messages(cfg, []refresh.Result{{Provider: "eps", OK: true, New: true, DurationMS: 3400}})

	require.Len(t, msgs, 1)
	assert.Equal(t, "Račun za EPS je uspešno preuzet.\n", msgs[0].Body)
}

func TestMessagesAllDoneRequiresEveryProvider(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModeAllDone}

	// One provider failed → no summary.
	assert.Empty(t, notify.Messages(cfg, []refresh.Result{
		{Provider: "mts", OK: true, New: true},
		{Provider: "a1", OK: false},
	}))

	// All succeeded but nothing was new (a daily re-run) → no summary.
	assert.Empty(t, notify.Messages(cfg, []refresh.Result{
		{Provider: "mts", OK: true, New: false},
		{Provider: "a1", OK: true, New: false},
	}))

	// All succeeded and at least one new → one summary.
	msgs := notify.Messages(cfg, []refresh.Result{
		{Provider: "mts", OK: true, New: true},
		{Provider: "a1", OK: true, New: false},
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

func TestHandleDueRemindersSummary(t *testing.T) {
	var sent []struct{ subject, body string }
	sender := senderFunc(func(subject, body string) error {
		sent = append(sent, struct{ subject, body string }{subject, body})
		return nil
	})

	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.Local)
	inTwoDays := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local).Unix()

	svc := notify.New(config.Notifications{
		Mode:         config.NotifyModeOff,
		Driver:       config.NotifyDriverTelegram,
		DueReminders: true,
	}, sender, zerolog.Nop())

	announced := svc.HandleDueReminders([]notify.DueReceipt{
		{Provider: "mts", Period: "06/2026", Price: 1819.46, DueAt: inTwoDays},
		{Provider: "eps", Period: "06/2026", Price: 2364.66, DueAt: inTwoDays},
		{Provider: "a1", Period: "06/2026", Price: 4215.88, DueAt: inTwoDays},
	}, now)

	require.Len(t, announced, 3)
	require.Len(t, sent, 1)
	assert.True(t, strings.HasPrefix(sent[0].subject, "⏰ "))
	assert.Contains(t, sent[0].body, "3 računa dospevaju za 2 dana")
	assert.Contains(t, sent[0].body, "8.400,00 RSD")
}

func TestHandleDueRemindersSilentWhenOff(t *testing.T) {
	var count int
	sender := senderFunc(func(string, string) error { count++; return nil })
	svc := notify.New(config.Notifications{Mode: config.NotifyModePerReceipt, Driver: config.NotifyDriverTelegram}, sender, zerolog.Nop())

	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.Local)
	due := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local).Unix()
	assert.Empty(t, svc.HandleDueReminders([]notify.DueReceipt{
		{Provider: "mts", DueAt: due, Price: 100},
	}, now))
	assert.Equal(t, 0, count)
}

func TestFilterDueRemindersWindow(t *testing.T) {
	now := time.Date(2026, 7, 29, 15, 0, 0, 0, time.Local)
	items := []notify.DueReceipt{
		{Provider: "overdue", DueAt: time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local).Unix()},
		{Provider: "today", DueAt: time.Date(2026, 7, 29, 0, 0, 0, 0, time.Local).Unix()},
		{Provider: "in3", DueAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local).Unix()},
		{Provider: "in4", DueAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.Local).Unix()},
		{Provider: "none", DueAt: 0},
	}

	got := notify.FilterDueReminders(items, now)
	require.Len(t, got, 3)
	assert.Equal(t, "overdue", got[0].Provider)
	assert.Equal(t, "today", got[1].Provider)
	assert.Equal(t, "in3", got[2].Provider)
}

func TestMessagesOffSendsNothing(t *testing.T) {
	assert.Empty(t, notify.Messages(config.Notifications{Mode: config.NotifyModeOff}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
	assert.Empty(t, notify.Messages(config.Notifications{}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
}
