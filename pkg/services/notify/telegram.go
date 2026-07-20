package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
)

const defaultTelegramAPI = "https://api.telegram.org"

var _ Sender = &TelegramSender{}

// TelegramSender delivers notifications via the Bot API sendMessage method.
type TelegramSender struct {
	api    string
	token  string
	chatID string
	http   *http.Client
}

func NewTelegram(cfg config.Telegram) (*TelegramSender, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("telegram bot_token is empty")
	}
	if cfg.ChatID == "" {
		return nil, fmt.Errorf("telegram chat_id is empty")
	}

	return &TelegramSender{
		api:    defaultTelegramAPI,
		token:  cfg.BotToken,
		chatID: cfg.ChatID,
		http:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// SetBaseURLForTest overrides the Telegram API host. Tests only.
func (t *TelegramSender) SetBaseURLForTest(base string) {
	t.api = base
}

func (t *TelegramSender) Send(subject, body string) error {
	text := subject
	if body != "" {
		text = subject + "\n\n" + body
	}

	payload, err := json.Marshal(map[string]string{
		"chat_id": t.chatID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", t.api, t.token)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("telegram: bad response (%s): %w", resp.Status, err)
	}
	if !parsed.OK {
		if parsed.Description == "" {
			parsed.Description = resp.Status
		}
		return fmt.Errorf("telegram: %s", parsed.Description)
	}

	return nil
}
