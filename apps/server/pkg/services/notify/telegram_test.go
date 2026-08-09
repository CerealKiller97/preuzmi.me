package notify_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramSendPostsSendMessage(t *testing.T) {
	var gotBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/botTOKEN/sendMessage", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	sender, err := notify.NewTelegram(config.Telegram{
		BotToken: "TOKEN",
		ChatID:   "42",
	})
	require.NoError(t, err)

	// Point the client at the fake API by swapping the package constant is hard;
	// instead exercise Send against a custom transport by using the public API
	// of NewTelegram and rewriting the request via a custom RoundTripper.
	sender.SetBaseURLForTest(server.URL)

	err = sender.Send("hello", "world")
	require.NoError(t, err)
	assert.Equal(t, "42", gotBody["chat_id"])
	assert.Equal(t, "hello\n\nworld", gotBody["text"])
}

func TestTelegramSendReportsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer server.Close()

	sender, err := notify.NewTelegram(config.Telegram{BotToken: "TOKEN", ChatID: "1"})
	require.NoError(t, err)
	sender.SetBaseURLForTest(server.URL)

	err = sender.Send("x", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat not found")
}
