package notify_test

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessagesPerReceipt(t *testing.T) {
	cfg := config.Notifications{Mode: config.NotifyModePerReceipt}
	results := []refresh.Result{
		{Provider: "mts", OK: true, DurationMS: 1200},
		{Provider: "a1", OK: false, Error: "login failed"},
	}

	msgs := notify.Messages(cfg, results)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, "MTS")
	assert.Contains(t, msgs[0].Body, "MTS")
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

func TestMessagesOffSendsNothing(t *testing.T) {
	assert.Empty(t, notify.Messages(config.Notifications{Mode: config.NotifyModeOff}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
	assert.Empty(t, notify.Messages(config.Notifications{}, []refresh.Result{
		{Provider: "mts", OK: true},
	}))
}
