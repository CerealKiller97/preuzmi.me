package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

// DefaultCheckUntil is used when check_until is omitted from config.json.
const DefaultCheckUntil = 20

// Allowed values for the top-level "storage" key.
const (
	StorageLocal = "local"
	StorageS3    = "s3"
)

type (
	Provider    string
	Credentials struct {
		Username string `json:"identifier"`
		Password string `json:"password"`
		// Mailbox is optional and only used by email-based providers (eUpravnik,
		// Yettel). It names the IMAP folder — a Gmail label works here — that
		// holds this provider's bills, so each provider can search its own label
		// instead of the shared config.Email.Mailbox. Empty falls back to
		// config.Email.Mailbox, then INBOX.
		Mailbox string `json:"mailbox,omitempty"`
	}

	// S3 configures any S3-compatible object storage (AWS S3, Linode Object
	// Storage, MinIO, ...). Endpoint may be left empty for AWS S3 proper; for
	// other providers it names the service host, e.g.
	// "eu-central-1.linodeobjects.com" or "localhost:9000".
	S3 struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		Region    string `json:"region"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}

	// SMTP is the email delivery settings used when notifications.driver is
	// "smtp". Port 587 uses STARTTLS; 465 uses implicit TLS.
	SMTP struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
		To       string `json:"to"`
		Port     int    `json:"port"`
	}

	// Telegram holds Bot API credentials used when notifications.driver is
	// "telegram".
	Telegram struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}

	// Email selects the IMAP server used by providers that deliver their
	// documents only by email (e.g. eUpravnik). It carries no secrets: the
	// mailbox login and app password are stored as that provider's own
	// credentials under "providers". Provider is one of gmail|outlook|yahoo|
	// icloud|custom; Host/Port are only needed for "custom" (or to override a
	// preset), and Mailbox defaults to INBOX.
	Email struct {
		Provider string `json:"provider"`
		Host     string `json:"host"`
		Mailbox  string `json:"mailbox"`
		Port     int    `json:"port"`
	}

	// Notifications controls whether (and how) the user is told about
	// completed downloads. Mode is mutually exclusive: only one behaviour
	// can be active at a time.
	Notifications struct {
		// Mode selects when to notify about downloads. Allowed:
		// off|per_receipt|all_done.
		Mode   string `json:"mode"`
		Driver string `json:"driver"`
		// PaidConfirmation, when true, sends a message every time a provider
		// newly confirms a receipt as paid ("plaćeno"). It is independent of
		// Mode: paid-confirmation messages can fire even when Mode is off, as
		// long as the driver is configured.
		PaidConfirmation bool     `json:"paid_confirmation"`
		SMTP             SMTP     `json:"smtp"`
		Telegram         Telegram `json:"telegram"`
	}

	Providers struct {
		MTS       Credentials `json:"mts"`
		A1        Credentials `json:"a1"`
		Yettel    Credentials `json:"yettel"`
		EPS       Credentials `json:"eps"`
		Esanduce  Credentials `json:"esanduce"`
		EUpravnik Credentials `json:"eupravnik"`
	}

	Config struct {
		Providers     map[Provider]Credentials `json:"providers"`
		Notifications Notifications            `json:"notifications"`
		Email         Email                    `json:"email"`
		S3            S3                       `json:"s3"`
		Application   struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"application"`
		Storage      string `json:"storage"`
		DownloadPath string `json:"download_path"`
		LogLevel     string `json:"log_level"`
		CheckUntil   int    `json:"check_until"`
		PrettyPrint  bool   `json:"pretty_print"`
	}
)

// Allowed values for notifications.driver.
const (
	NotifyDriverSMTP     = "smtp"
	NotifyDriverTelegram = "telegram"
)

// Allowed values for notifications.mode.
const (
	NotifyModeOff        = "off"
	NotifyModePerReceipt = "per_receipt"
	NotifyModeAllDone    = "all_done"
)

// Allowed values for email.provider. "custom" (or an empty value) means the
// IMAP host/port are given explicitly.
var emailProviders = map[string]struct{}{
	"":        {},
	"gmail":   {},
	"outlook": {},
	"yahoo":   {},
	"icloud":  {},
	"custom":  {},
}

// SecretPlaceholder is what the settings UI sends back for an unchanged secret.
const SecretPlaceholder = "••••••••"

// Path returns the absolute path of config.json in the process working directory.
func Path() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return path.Join(cwd, "config.json"), nil
}

func New() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}

	return Load(p)
}

// Load reads and validates config from path.
func Load(path string) (Config, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o600)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	cfg := Config{}

	data, err := io.ReadAll(f)
	if err != nil {
		return Config{}, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Save writes cfg to path atomically (temp file + rename).
func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// KeepSecret returns current when incoming is empty or the UI placeholder,
// otherwise the new value. Lets the settings form omit unchanged secrets.
func KeepSecret(incoming, current string) string {
	if incoming == "" || incoming == SecretPlaceholder {
		return current
	}

	return incoming
}

// Validate checks the storage configuration. An empty "storage" value falls
// back to local so existing config files keep working unchanged.
func (c *Config) Validate() error {
	if c.Application.Port < 0 || c.Application.Port > 65535 {
		return fmt.Errorf("application.port must be between 0 and 65535, got %d", c.Application.Port)
	}

	switch c.Storage {
	case "":
		c.Storage = StorageLocal
	case StorageLocal:
	case StorageS3:
		if c.S3.Bucket == "" {
			return errors.New("storage is s3 but s3.bucket is empty")
		}
		if c.S3.AccessKey == "" || c.S3.SecretKey == "" {
			return errors.New("storage is s3 but s3.access_key or s3.secret_key is empty")
		}
	default:
		return fmt.Errorf("invalid storage %q, allowed values are %q and %q", c.Storage, StorageLocal, StorageS3)
	}

	if c.DownloadPath == "" {
		return errors.New("download_path is empty")
	}

	if c.CheckUntil == 0 {
		c.CheckUntil = DefaultCheckUntil
	}
	if c.CheckUntil < 1 || c.CheckUntil > 31 {
		return fmt.Errorf("check_until must be between 1 and 31, got %d", c.CheckUntil)
	}

	switch c.LogLevel {
	case "":
		c.LogLevel = "info"
	case "trace", "debug", "info", "warn", "error", "fatal", "panic", "disabled":
	default:
		return fmt.Errorf("invalid log_level %q", c.LogLevel)
	}

	if err := c.validateNotifications(); err != nil {
		return err
	}

	if err := c.validateEmail(); err != nil {
		return err
	}

	return nil
}

// validateEmail checks the IMAP server selection. A custom (or empty) provider
// needs an explicit host; a named provider must be one we have a preset for.
func (c *Config) validateEmail() error {
	provider := strings.ToLower(strings.TrimSpace(c.Email.Provider))

	if _, ok := emailProviders[provider]; !ok {
		return fmt.Errorf("invalid email.provider %q", c.Email.Provider)
	}

	if c.Email.Port < 0 || c.Email.Port > 65535 {
		return fmt.Errorf("email.port must be between 0 and 65535, got %d", c.Email.Port)
	}

	return nil
}

// MergeSecrets copies secret fields from prev whenever next still holds the
// UI placeholder or an empty string, so applying settings never blanks a
// password the user did not intentionally replace.
func (next *Config) MergeSecrets(prev Config) {
	next.S3.AccessKey = KeepSecret(next.S3.AccessKey, prev.S3.AccessKey)
	next.S3.SecretKey = KeepSecret(next.S3.SecretKey, prev.S3.SecretKey)
	next.Notifications.SMTP.Password = KeepSecret(next.Notifications.SMTP.Password, prev.Notifications.SMTP.Password)
	next.Notifications.Telegram.BotToken = KeepSecret(next.Notifications.Telegram.BotToken, prev.Notifications.Telegram.BotToken)

	if next.Providers == nil {
		next.Providers = map[Provider]Credentials{}
	}

	for name, prevCreds := range prev.Providers {
		cur := next.Providers[name]
		cur.Password = KeepSecret(cur.Password, prevCreds.Password)
		next.Providers[name] = cur
	}
}

// validateNotifications only requires delivery settings when mode is not off.
func (c *Config) validateNotifications() error {
	n := c.Notifications

	switch n.Mode {
	case "":
		c.Notifications.Mode = NotifyModeOff
		n.Mode = NotifyModeOff
	case NotifyModeOff, NotifyModePerReceipt, NotifyModeAllDone:
	default:
		return fmt.Errorf(
			"invalid notifications.mode %q, allowed values are %q, %q and %q",
			n.Mode, NotifyModeOff, NotifyModePerReceipt, NotifyModeAllDone,
		)
	}

	// A driver is only required when something will actually be delivered:
	// an active download mode, or paid-confirmation messages.
	if !n.NotifyEnabled() && !n.PaidConfirmation {
		return nil
	}

	switch n.Driver {
	case "":
		c.Notifications.Driver = NotifyDriverSMTP
		n.Driver = NotifyDriverSMTP
	case NotifyDriverSMTP:
	case NotifyDriverTelegram:
	default:
		return fmt.Errorf("invalid notifications.driver %q, allowed values are %q and %q", n.Driver, NotifyDriverSMTP, NotifyDriverTelegram)
	}

	if n.Driver == NotifyDriverSMTP {
		if n.SMTP.Host == "" {
			return errors.New("notifications are enabled but notifications.smtp.host is empty")
		}
		if n.SMTP.Port == 0 {
			c.Notifications.SMTP.Port = 587
		}
		if n.SMTP.From == "" || n.SMTP.To == "" {
			return errors.New("notifications are enabled but notifications.smtp.from or smtp.to is empty")
		}
	}

	if n.Driver == NotifyDriverTelegram {
		if n.Telegram.BotToken == "" || n.Telegram.ChatID == "" {
			return errors.New("notifications are enabled but notifications.telegram.bot_token or telegram.chat_id is empty")
		}
	}

	return nil
}

// NotifyEnabled reports whether any automatic download-notification mode is
// active.
func (n Notifications) NotifyEnabled() bool {
	return n.Mode == NotifyModePerReceipt || n.Mode == NotifyModeAllDone
}

// DeliveryEnabled reports whether any message will be sent at all: a download
// mode, paid-confirmation, or both. It governs when a driver must be configured.
func (n Notifications) DeliveryEnabled() bool {
	return n.NotifyEnabled() || n.PaidConfirmation
}

// RefreshAllowed reports whether today is still within the check_until window
// for the current month.
func (c *Config) RefreshAllowed() bool {
	return c.allowedOnDay(time.Now().Day())
}

// allowedOnDay reports whether a run is allowed on the given day of the month.
// The window is inclusive of check_until: with check_until 20, day 20 still runs
// and day 21 does not.
func (c *Config) allowedOnDay(day int) bool {
	return day <= c.CheckUntil
}
