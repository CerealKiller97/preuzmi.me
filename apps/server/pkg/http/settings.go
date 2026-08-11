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
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/rs/zerolog/log"
)

// redacted replaces every secret before it leaves the process.
const redacted = config.SecretPlaceholder

// redactedAccount is one provider account for display: its id/label plus the
// identifier, with the password shown only as the placeholder when set.
//
// The identifier is kept so the account in use can be verified at a glance, but
// the password never leaves the server: this endpoint has no authentication and
// the app binds to whatever host config.json names, which may well be every
// interface on the machine.
type redactedAccount struct {
	ID         string `json:"id,omitempty"`
	Label      string `json:"label,omitempty"`
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
	Mailbox    string `json:"mailbox,omitempty"`
}

// redactedConfig mirrors config.Config for display.
//
// It is built from the parsed configuration rather than by re-reading the file,
// so what is shown is what the application is actually using. Keys present in
// config.json but absent from config.Config are silently ignored at load time
// and therefore will not appear here either.
type redactedConfig struct {
	Providers     map[string][]redactedAccount `json:"providers"`
	Notifications redactedNotifications        `json:"notifications"`
	Email         redactedEmail                  `json:"email"`
	S3            redactedS3                     `json:"s3"`
	Application   struct {
		Host  string `json:"host"`
		Port  int    `json:"port"`
		Certs struct {
			Certificate string `json:"cert"`
			PrivateKey  string `json:"key"`
		} `json:"certs"`
	} `json:"application"`
	Storage      string `json:"storage"`
	DownloadPath string `json:"download_path"`
	LogLevel     string `json:"log_level"`
	Lang         string `json:"lang"`
	CheckUntil   int    `json:"check_until"`
	PrettyPrint  bool   `json:"pretty_print"`
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
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	Port     int    `json:"port"`
}

type redactedTelegram struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// redactedEmail mirrors config.Email. It holds no secret — the mailbox login and
// app password live under providers — so every field is shown as-is.
type redactedEmail struct {
	Provider string `json:"provider"`
	Host     string `json:"host"`
	Mailbox  string `json:"mailbox"`
	Port     int    `json:"port"`
}

type redactedNotifications struct {
	Mode             string           `json:"mode"`
	Driver           string           `json:"driver"`
	PaidConfirmation bool             `json:"paid_confirmation"`
	DueReminders     bool             `json:"due_reminders"`
	DueReminderDays  int              `json:"due_reminder_days"`
	SMTP             redactedSMTP     `json:"smtp"`
	Telegram         redactedTelegram `json:"telegram"`
}

// redact builds the display copy of the configuration.
func redact(cfg *config.Config) redactedConfig {
	out := redactedConfig{
		Storage:      cfg.Storage,
		DownloadPath: cfg.DownloadPath,
		CheckUntil:   cfg.CheckUntil,
		LogLevel:     cfg.LogLevel,
		Lang:         cfg.Lang,
		PrettyPrint:  cfg.PrettyPrint,
		Providers:    make(map[string][]redactedAccount, len(cfg.Providers)),
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
	out.Notifications.PaidConfirmation = cfg.Notifications.PaidConfirmation
	out.Notifications.DueReminders = cfg.Notifications.DueReminders
	out.Notifications.DueReminderDays = cfg.Notifications.DueReminderDays
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

	out.Email.Provider = cfg.Email.Provider
	out.Email.Host = cfg.Email.Host
	out.Email.Mailbox = cfg.Email.Mailbox
	out.Email.Port = cfg.Email.Port

	out.Application.Host = cfg.Application.Host
	out.Application.Port = cfg.Application.Port

	for name, accounts := range cfg.Providers {
		list := make([]redactedAccount, 0, len(accounts))
		for _, a := range accounts {
			entry := redactedAccount{
				ID:         a.ID,
				Label:      a.Label,
				Identifier: a.Username,
				Mailbox:    a.Mailbox,
			}
			if a.Password != "" {
				entry.Password = redacted
			}
			list = append(list, entry)
		}
		out.Providers[string(name)] = list
	}

	return out
}

// editableForm is the JSON shape the settings UI edits. Secret fields are
// blank so the browser never sees them; an empty submit keeps the previous
// value via MergeSecrets.
func editableForm(cfg *config.Config) config.Config {
	out := *cfg
	out.S3.AccessKey = ""
	out.S3.SecretKey = ""
	out.Notifications.SMTP.Password = ""
	out.Notifications.Telegram.BotToken = ""

	out.Providers = make(map[config.Provider]config.ProviderAccounts, len(cfg.Providers))
	for name, accounts := range cfg.Providers {
		list := make(config.ProviderAccounts, 0, len(accounts))
		for _, a := range accounts {
			list = append(list, config.Account{
				ID:       a.ID,
				Label:    a.Label,
				Username: a.Username,
				Password: "",
				// Mailbox is not a secret; carry it through so a settings save from
				// the UI does not blank a provider's configured folder/label.
				Mailbox: a.Mailbox,
			})
		}
		out.Providers[name] = list
	}

	return out
}

// ProviderStatus summarises one provider account for the settings table. A solo
// provider yields a single entry with an empty Account/Label; a multi-account
// provider yields one entry per account.
type ProviderStatus struct {
	Name        string
	Account     string
	Label       string
	Identifier  string
	HasPassword bool
	// Configured means both halves of the credential pair are present.
	Configured bool
	// Implemented means a download actually exists for this provider.
	Implemented bool
}

// providerStatuses builds one status entry per configured account across all
// providers, sorted by provider then account so the settings table is stable.
func providerStatuses(cfg *config.Config) []ProviderStatus {
	out := make([]ProviderStatus, 0, len(cfg.Providers))
	for name, accounts := range cfg.Providers {
		for _, a := range accounts {
			out = append(out, ProviderStatus{
				Name:        string(name),
				Account:     a.ID,
				Label:       a.Label,
				Identifier:  a.Username,
				HasPassword: a.Password != "",
				Configured:  a.Configured(),
				Implemented: container.IsImplemented(string(name)),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}

		return out[i].Account < out[j].Account
	})

	return out
}

// providerStatusKey is the map key used to expose per-account secret presence to
// the settings UI: the provider name for a solo account, or "provider/account"
// for a named one.
func providerStatusKey(name, account string) string {
	if account == "" {
		return name
	}

	return name + "/" + account
}

// SettingsPageData backs the settings template.
type SettingsPageData struct {
	Config redactedConfig
	PageData
	FormJSON             template.JS
	ConfigPath           string
	Storage              string
	StorageTarget        string
	ProviderSecretsJSON  template.JS
	Providers            []ProviderStatus
	IgnoredKeys          []string
	ReceiptCount         int
	DownloadPathWritable bool
	HasAppKey            bool
	HasS3Access          bool
	HasS3Secret          bool
	HasSMTPPass          bool
	HasTgToken           bool
	NotifyCanTest        bool
	DownloadPathExists   bool
}

// knownKeys are the top-level config.json keys config.Config actually parses.
var knownKeys = map[string]struct{}{
	"application":   {},
	"storage":       {},
	"s3":            {},
	"download_path": {},
	"check_until":   {},
	"lang":          {},
	"notifications": {},
	"email":         {},
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

// APIProviderStatus is the JSON-friendly provider summary for GET /api/settings.
type APIProviderStatus struct {
	Name string `json:"name"`
	// Account is the provider account id ("" for a solo provider); Label is its
	// display name. Both are omitted for solo so a single-account payload is
	// unchanged from before multi-account support.
	Account     string `json:"account,omitempty"`
	Label       string `json:"label,omitempty"`
	Identifier  string `json:"identifier"`
	HasPassword bool   `json:"has_password"`
	Configured  bool   `json:"configured"`
	Implemented bool   `json:"implemented"`
}

// APISettings is the JSON the mobile client reads to render and edit settings.
// Form is the editable config (secrets blanked) that goes straight back to
// PUT /api/settings; the has_* flags say which secrets already exist so the UI
// can show a "leave blank to keep" hint instead of wiping them. It mirrors the
// data the server-rendered settings page (settingsHandler) puts on screen.
type APISettings struct {
	Form                 config.Config       `json:"form"`
	Version              string              `json:"version"`
	Storage              string              `json:"storage"`
	StorageTarget        string              `json:"storage_target"`
	DownloadPath         string              `json:"download_path"`
	Providers            []APIProviderStatus `json:"providers"`
	IgnoredKeys          []string            `json:"ignored_keys"`
	ReceiptCount         int                 `json:"receipt_count"`
	DownloadPathExists   bool                `json:"download_path_exists"`
	DownloadPathWritable bool                `json:"download_path_writable"`
	NotifyCanTest        bool                `json:"notify_can_test"`
	HasS3Access          bool                `json:"has_s3_access"`
	HasS3Secret          bool                `json:"has_s3_secret"`
	HasSMTPPass          bool                `json:"has_smtp_pass"`
	HasTgToken           bool                `json:"has_tg_token"`
}

// settingsAPIHandler returns the current settings as JSON for the mobile client.
// It reuses the same redaction/edit-form helpers as the HTML settings page so
// the two never drift: Form comes from editableForm (secrets blanked), and the
// status fields mirror SettingsPageData.
func settingsAPIHandler(cfg *config.Config, version string, notifier *notify.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses := providerStatuses(cfg)
		providers := make([]APIProviderStatus, 0, len(statuses))
		for _, s := range statuses {
			providers = append(providers, APIProviderStatus(s))
		}

		dir := cfg.DownloadPath
		_, statErr := os.Stat(dir)
		exists := statErr == nil

		receipts, err := scanReceipts(cfg, nil)
		if err != nil {
			log.Err(err).Msg("Error counting receipts for settings API")
		}

		storageTarget := cfg.DownloadPath
		if cfg.Storage == config.StorageS3 {
			storageTarget = cfg.S3.Bucket
		}

		wd, _ := os.Getwd()
		configPath := path.Join(wd, "config.json")

		writeJSON(w, APISettings{
			Form:                 editableForm(cfg),
			Version:              version,
			Storage:              cfg.Storage,
			StorageTarget:        storageTarget,
			DownloadPath:         cfg.DownloadPath,
			Providers:            providers,
			IgnoredKeys:          ignoredConfigKeys(configPath),
			ReceiptCount:         len(receipts),
			DownloadPathExists:   exists,
			DownloadPathWritable: exists && writable(dir),
			NotifyCanTest:        notifier.CanTest(),
			HasS3Access:          cfg.S3.AccessKey != "",
			HasS3Secret:          cfg.S3.SecretKey != "",
			HasSMTPPass:          cfg.Notifications.SMTP.Password != "",
			HasTgToken:           cfg.Notifications.Telegram.BotToken != "",
		})
	}
}

func settingsHandler(cfg *config.Config, version string, notifier *notify.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {

		tmpl, err := parseTemplates(
			cfg.Lang,
			"./templates/settings.html",
			"./templates/partials.html",
		)
		if err != nil {
			log.Err(err).Msg("Error parsing settings template")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		// One row per account, stably ordered (ranging a map would reshuffle).
		providers := providerStatuses(cfg)

		dir := cfg.DownloadPath
		_, statErr := os.Stat(dir)
		exists := statErr == nil

		receipts, err := scanReceipts(cfg, nil)
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
			providerSecrets[providerStatusKey(p.Name, p.Account)] = p.HasPassword
		}
		providerSecretsJSON, _ := json.Marshal(providerSecrets)

		viewModel := SettingsPageData{
			PageData: PageData{
				URL:         "/settings",
				Active:      "settings",
				Title:       "Preuzmi.me — Podešavanja",
				Description: "Trenutna konfiguracija aplikacije.",
				BaseURL:     baseURL(r),
				Version:     version,
			},
			ConfigPath:           filepath.Clean(configPath),
			Config:               redact(cfg),
			Providers:            providers,
			ReceiptCount:         len(receipts),
			Storage:              cfg.Storage,
			StorageTarget:        storageTarget,
			DownloadPathExists:   exists,
			DownloadPathWritable: exists && writable(dir),
			NotifyCanTest:        notifier.CanTest(),
			FormJSON:             template.JS(formJSON),
			HasS3Access:          cfg.S3.AccessKey != "",
			HasS3Secret:          cfg.S3.SecretKey != "",
			HasSMTPPass:          cfg.Notifications.SMTP.Password != "",
			HasTgToken:           cfg.Notifications.Telegram.BotToken != "",
			ProviderSecretsJSON:  template.JS(providerSecretsJSON),
			IgnoredKeys:          ignoredConfigKeys(configPath),
		}
		withPageScript(&viewModel.PageData, cfg.Lang)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderTemplate(w, tmpl, viewModel)
	}
}
