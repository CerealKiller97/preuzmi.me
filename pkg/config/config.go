package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from"`
		To       string `json:"to"`
	}

	// Telegram holds Bot API credentials used when notifications.driver is
	// "telegram".
	Telegram struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}

	// Notifications controls whether (and how) the user is told about
	// completed downloads. Mode is mutually exclusive: only one behaviour
	// can be active at a time.
	Notifications struct {
		// Mode selects when to notify. Allowed: off|per_receipt|all_done.
		Mode     string   `json:"mode"`
		Driver   string   `json:"driver"`
		SMTP     SMTP     `json:"smtp"`
		Telegram Telegram `json:"telegram"`
	}

	Providers struct {
		MTS      Credentials `json:"mts"`
		A1       Credentials `json:"a1"`
		Yettel   Credentials `json:"yettel"`
		EPS      Credentials `json:"eps"`
		Esanduce Credentials `json:"esanduce"`
	}

	Config struct {
		Application struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			Certs struct {
				Certificate string `json:"cert"`
				PrivateKey  string `json:"key"`
			} `json:"certs"`
		} `json:"application"`
		Storage      string                   `json:"storage"` // Allowed values: local|s3
		DownloadPath string                   `json:"download_path"`
		S3           S3                       `json:"s3"`
		// CheckUntil is the last day of the month when a refresh is useful.
		// Providers stop issuing new receipts after this day, so the UI
		// disables the refresh button and the API rejects refreshes past it.
		CheckUntil      int                      `json:"check_until"`
		Notifications   Notifications            `json:"notifications"`
		LogLevel        string                   `json:"log_level"`
		PrettyPrint     bool                     `json:"pretty_print"`
		Providers       map[Provider]Credentials `json:"providers"`
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

func New() (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	f, err := os.OpenFile(path.Join(cwd, "config.json"), os.O_RDONLY, 0o600)
	if err != nil {
		return Config{}, err
	}

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

// Validate checks the storage configuration. An empty "storage" value falls
// back to local so existing config files keep working unchanged.
func (c *Config) Validate() error {
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

	if c.CheckUntil == 0 {
		c.CheckUntil = DefaultCheckUntil
	}
	if c.CheckUntil < 1 || c.CheckUntil > 31 {
		return fmt.Errorf("check_until must be between 1 and 31, got %d", c.CheckUntil)
	}

	if err := c.validateNotifications(); err != nil {
		return err
	}

	return nil
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

	if n.Mode == NotifyModeOff {
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

// NotifyEnabled reports whether any automatic notification mode is active.
func (n Notifications) NotifyEnabled() bool {
	return n.Mode == NotifyModePerReceipt || n.Mode == NotifyModeAllDone
}

// RefreshAllowed reports whether today is still within the check_until window
// for the current month.
func (c *Config) RefreshAllowed() bool {
	return time.Now().Day() <= c.CheckUntil
}
