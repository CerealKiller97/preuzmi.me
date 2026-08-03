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

// Provider keys are ASCII, so uppercasing them directly mangles the ones that
// should read "E-SANDUČE" / "E-UPRAVNIK" (→ "Е-САНДУЧЕ" / "Е-УПРАВНИК" in
// Cyrillic) instead of the run-together "ESANDUCE" / "EUPRAVNIK".
func TestMessagesProviderDisplayName(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	cases := []struct{ key, want, wrong string }{
		{"esanduce", "E-SANDUČE", "ESANDUCE"},
		{"eupravnik", "E-UPRAVNIK", "EUPRAVNIK"},
	}
	for _, tc := range cases {
		msgs := notify.Messages(cfg, []refresh.Result{
			{Provider: tc.key, OK: true, New: true, Period: "05-2026"},
		})
		require.Len(t, msgs, 1, tc.key)
		assert.Contains(t, msgs[0].Subject, tc.want)
		assert.Contains(t, msgs[0].Body, tc.want)
		assert.NotContains(t, msgs[0].Subject, tc.wrong)
	}
}

// Cyrillic messages resolve each provider name by its rule: custom spelling,
// kept-Latin brand, transliterated acronym, or transliterated Serbian word.
func TestSendCyrillicProviderNames(t *testing.T) {
	var got struct{ subject, body string }
	sender := senderFunc(func(subject, body string) error {
		got.subject, got.body = subject, body
		return nil
	})
	svc := notify.NewWithLang(
		config.Notifications{Mode: config.NotifyModePerReceipt, PaidConfirmation: true},
		sender, zerolog.Nop(), config.LangCyrillic,
	)

	// Yettel: custom Cyrillic spelling (not a transliteration of "YETTEL").
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "yettel", Period: "05-2026"}})
	assert.Contains(t, got.subject, "ЈЕТЕЛ")
	assert.Contains(t, got.body, "Рачун за ЈЕТЕЛ")
	assert.NotContains(t, got.body, "YETTEL")

	// A1: foreign brand kept Latin, surrounding text is Cyrillic.
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "a1", Period: "05-2026"}})
	assert.Contains(t, got.body, "Рачун за A1")

	// Serbian-word provider: name itself transliterates.
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "esanduce", Period: "05-2026"}})
	assert.Contains(t, got.subject, "Е-САНДУЧЕ")
	assert.NotContains(t, got.body, "E-SANDUČE")

	// Serbian acronym: transliterates like ordinary text (EPS → ЕПС).
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "eps", Period: "05-2026"}})
	assert.Contains(t, got.subject, "ЕПС")
	assert.NotContains(t, got.body, "EPS")
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

func TestHandleVerifiedSendsCyrillic(t *testing.T) {
	var sent []struct{ subject, body string }
	sender := senderFunc(func(subject, body string) error {
		sent = append(sent, struct{ subject, body string }{subject, body})
		return nil
	})

	svc := notify.NewWithLang(
		config.Notifications{Mode: config.NotifyModeOff, Driver: config.NotifyDriverTelegram, PaidConfirmation: true},
		sender,
		zerolog.Nop(),
		config.LangCyrillic,
	)
	svc.HandleVerified([]notify.VerifiedReceipt{{Provider: "mts", Period: "05-2026", Price: 1819.46}})

	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].subject, "рачун")
	assert.Contains(t, sent[0].subject, "потврђен")
	assert.Contains(t, sent[0].body, "плаћен")
	assert.NotContains(t, sent[0].body, "plaćen")
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
