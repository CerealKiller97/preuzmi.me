package http

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/rs/zerolog/log"
)

// redacted replaces every secret before it leaves the process.
const redacted = config.SecretPlaceholder

// redactedCredentials mirrors config.Credentials with the password removed.
//
// The identifier is kept so the account in use can be verified at a glance,
// but the password never leaves the server: this endpoint has no authentication
// and the app binds to whatever host config.json names, which may well be every
// interface on the machine.
type redactedCredentials struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// redactedConfig mirrors config.Config for display.
//
// It is built from the parsed configuration rather than by re-reading the file,
// so what is shown is what the application is actually using. Keys present in
// config.json but absent from config.Config are silently ignored at load time
// and therefore will not appear here either.
type redactedConfig struct {
	Application struct {
		Host  string `json:"host"`
		Port  int    `json:"port"`
		Certs struct {
			Certificate string `json:"cert"`
			PrivateKey  string `json:"key"`
		} `json:"certs"`
	} `json:"application"`
	Storage      string                         `json:"storage"`
	DownloadPath string                         `json:"download_path"`
	CheckUntil     int                            `json:"check_until"`
	S3             redactedS3                     `json:"s3"`
	Notifications  redactedNotifications          `json:"notifications"`
	LogLevel       string                         `json:"log_level"`
	PrettyPrint    bool                           `json:"pretty_print"`
	Providers      map[string]redactedCredentials `json:"providers"`
}

// redactedS3 mirrors config.S3 with both credential halves hidden: the access
// key alone already identifies the account on most S3-compatible services.
type redactedS3 struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

type redactedSMTP struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type redactedTelegram struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

type redactedNotifications struct {
	Mode     string           `json:"mode"`
	Driver   string           `json:"driver"`
	SMTP     redactedSMTP     `json:"smtp"`
	Telegram redactedTelegram `json:"telegram"`
}

// redact builds the display copy of the configuration.
func redact(cfg *config.Config) redactedConfig {
	out := redactedConfig{
		Storage:      cfg.Storage,
		DownloadPath: cfg.DownloadPath,
		CheckUntil:   cfg.CheckUntil,
		LogLevel:     cfg.LogLevel,
		PrettyPrint:  cfg.PrettyPrint,
		Providers:    make(map[string]redactedCredentials, len(cfg.Providers)),
	}

	out.S3.Endpoint = cfg.S3.Endpoint
	out.S3.Bucket = cfg.S3.Bucket
	out.S3.Region = cfg.S3.Region
	if cfg.S3.AccessKey != "" {
		out.S3.AccessKey = redacted
	}
	if cfg.S3.SecretKey != "" {
		out.S3.SecretKey = redacted
	}

	out.Notifications.Mode = cfg.Notifications.Mode
	out.Notifications.Driver = cfg.Notifications.Driver
	out.Notifications.SMTP.Host = cfg.Notifications.SMTP.Host
	out.Notifications.SMTP.Port = cfg.Notifications.SMTP.Port
	out.Notifications.SMTP.Username = cfg.Notifications.SMTP.Username
	out.Notifications.SMTP.From = cfg.Notifications.SMTP.From
	out.Notifications.SMTP.To = cfg.Notifications.SMTP.To
	if cfg.Notifications.SMTP.Password != "" {
		out.Notifications.SMTP.Password = redacted
	}
	if cfg.Notifications.Telegram.BotToken != "" {
		out.Notifications.Telegram.BotToken = redacted
	}
	out.Notifications.Telegram.ChatID = cfg.Notifications.Telegram.ChatID

	out.Application.Host = cfg.Application.Host
	out.Application.Port = cfg.Application.Port
	out.Application.Certs.Certificate = cfg.Application.Certs.Certificate

	// The certificate path is harmless, but the private key path points
	// straight at a secret, so treat it as one.
	if cfg.Application.Certs.PrivateKey != "" {
		out.Application.Certs.PrivateKey = redacted
	}

	for name, creds := range cfg.Providers {
		entry := redactedCredentials{Identifier: creds.Username}
		if creds.Password != "" {
			entry.Password = redacted
		}

		out.Providers[string(name)] = entry
	}

	return out
}

// editableForm is the JSON shape the settings UI edits. Secret fields are
// blank so the browser never sees them; an empty submit keeps the previous
// value via MergeSecrets.
func editableForm(cfg *config.Config) config.Config {
	out := *cfg
	out.Application.Certs.PrivateKey = ""
	out.S3.AccessKey = ""
	out.S3.SecretKey = ""
	out.Notifications.SMTP.Password = ""
	out.Notifications.Telegram.BotToken = ""

	out.Providers = make(map[config.Provider]config.Credentials, len(cfg.Providers))
	for name, creds := range cfg.Providers {
		out.Providers[name] = config.Credentials{
			Username: creds.Username,
			Password: "",
		}
	}

	return out
}

// ProviderStatus summarises one provider for the settings table.
type ProviderStatus struct {
	Name        string
	Identifier  string
	HasPassword bool
	// Configured means both halves of the credential pair are present.
	Configured bool
	// Implemented means a download actually exists for this provider.
	Implemented bool
}

// SettingsPageData backs the settings template.
type SettingsPageData struct {
	PageData

	ConfigPath string
	// Config is the redacted, in-memory configuration shown as form fields.
	Config       redactedConfig
	Providers    []ProviderStatus
	ReceiptCount int

	// Storage names the configured receipt storage driver ("local" or "s3").
	// StorageTarget is where receipts end up: the download path for local
	// storage, the bucket name for S3.
	Storage       string
	StorageTarget string

	// DownloadPathExists and DownloadPathWritable catch the most common
	// misconfiguration: a path that has moved or cannot be written to.
	DownloadPathExists   bool
	DownloadPathWritable bool

	// NotifyCanTest is true when the selected driver has enough credentials
	// to send a probe message from the settings page.
	NotifyCanTest bool

	// FormJSON is the editable config snapshot for Alpine (secrets blanked).
	FormJSON template.JS

	HasAppKey   bool
	HasS3Access bool
	HasS3Secret bool
	HasSMTPPass bool
	HasTgToken  bool
	// ProviderSecretsJSON maps provider name → whether a password is stored.
	ProviderSecretsJSON template.JS

	// IgnoredKeys are top-level keys present in config.json that the
	// application does not understand and therefore silently discards.
	IgnoredKeys []string
}

// knownKeys are the top-level config.json keys config.Config actually parses.
var knownKeys = map[string]struct{}{
	"application":   {},
	"storage":       {},
	"s3":            {},
	"download_path": {},
	"check_until":   {},
	"notifications": {},
	"log_level":     {},
	"pretty_print":  {},
	"providers":     {},
}

// ignoredConfigKeys reports top-level keys in the file that are thrown away on
// load, so a setting that looks configured but does nothing is visible rather
// than silently ineffective.
func ignoredConfigKeys(configPath string) []string {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}

	ignored := make([]string, 0)
	for key := range raw {
		if _, ok := knownKeys[key]; !ok {
			ignored = append(ignored, key)
		}
	}

	sort.Strings(ignored)

	return ignored
}

// writable reports whether dir can be written to, by creating and removing a
// probe file. Checking permission bits alone would miss read-only mounts.
func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return false
	}

	name := probe.Name()
	probe.Close()

	return os.Remove(name) == nil
}

func settingsHandler(c *container.Container) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := c.GetConfig()

		tmpl, err := template.ParseFiles(
			"./templates/settings.html",
			"./templates/partials.html",
		)
		if err != nil {
			log.Err(err).Msg("Error parsing settings template")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		// Stable order: ranging a map would reshuffle the table every reload.
		names := make([]string, 0, len(cfg.Providers))
		for name := range cfg.Providers {
			names = append(names, string(name))
		}
		sort.Strings(names)

		providers := make([]ProviderStatus, 0, len(names))
		for _, name := range names {
			creds := cfg.Providers[config.Provider(name)]
			providers = append(providers, ProviderStatus{
				Name:        name,
				Identifier:  creds.Username,
				HasPassword: creds.Password != "",
				Configured:  creds.Username != "" && creds.Password != "",
				Implemented: container.IsImplemented(name),
			})
		}

		dir := cfg.DownloadPath
		_, statErr := os.Stat(dir)
		exists := statErr == nil

		receipts, err := scanReceipts(dir, nil)
		if err != nil {
			log.Err(err).Msg("Error counting receipts for settings")
		}

		wd, _ := os.Getwd()
		configPath := path.Join(wd, "config.json")

		storageTarget := cfg.DownloadPath
		if cfg.Storage == config.StorageS3 {
			storageTarget = cfg.S3.Bucket
		}

		formJSON, err := json.Marshal(editableForm(cfg))
		if err != nil {
			log.Err(err).Msg("Error encoding settings form JSON")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Prevent </script> in paths from breaking the JSON script tag.
		formJSON = bytes.ReplaceAll(formJSON, []byte("<"), []byte(`\u003c`))
		formJSON = bytes.ReplaceAll(formJSON, []byte(">"), []byte(`\u003e`))
		formJSON = bytes.ReplaceAll(formJSON, []byte("&"), []byte(`\u0026`))

		providerSecrets := map[string]bool{}
		for _, p := range providers {
			providerSecrets[p.Name] = p.HasPassword
		}
		providerSecretsJSON, _ := json.Marshal(providerSecrets)

		viewModel := SettingsPageData{
			PageData: PageData{
				URL:         "/settings",
				Active:      "settings",
				Title:       "Preuzmi.me — Podešavanja",
				Description: "Trenutna konfiguracija aplikacije.",
				BaseURL:     baseURL(r),
				Version:     c.GetVersion(),
			},
			ConfigPath:           filepath.Clean(configPath),
			Config:               redact(cfg),
			Providers:            providers,
			ReceiptCount:         len(receipts),
			Storage:              cfg.Storage,
			StorageTarget:        storageTarget,
			DownloadPathExists:   exists,
			DownloadPathWritable: exists && writable(dir),
			NotifyCanTest:        c.GetNotifier().CanTest(),
			FormJSON:             template.JS(formJSON),
			HasAppKey:            cfg.Application.Certs.PrivateKey != "",
			HasS3Access:          cfg.S3.AccessKey != "",
			HasS3Secret:          cfg.S3.SecretKey != "",
			HasSMTPPass:          cfg.Notifications.SMTP.Password != "",
			HasTgToken:           cfg.Notifications.Telegram.BotToken != "",
			ProviderSecretsJSON:  template.JS(providerSecretsJSON),
			IgnoredKeys:          ignoredConfigKeys(configPath),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err := tmpl.Execute(w, viewModel); err != nil {
			log.Err(err).Msg("Error executing settings template")
		}
	}
}
