<p align="center">
  <img src="assets/img/og.png" alt="Preuzmi.me — automatic receipt downloads" width="880" />
</p>

<h1 align="center">Preuzmi.me</h1>

<p align="center">
  <strong>English</strong> · <a href="README.sr.md">Srpski</a>
</p>

<p align="center">
  <strong>Automatic receipt downloads</strong> for internet, phone, and electricity — in one place.<br/>
  Dashboard, spending stats, notifications, and settings that hot-reload without a restart.
</p>

<p align="center">
  <a href="#features">Features</a> ·
  <a href="#screenshots">Screenshots</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#configuration">Config</a> ·
  <a href="#cli">CLI</a>
</p>

<p align="center">
  <a href="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/go-vet.yml"><img alt="go vet" src="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/go-vet.yml/badge.svg" /></a>
  <a href="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/go-test.yml"><img alt="go test" src="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/go-test.yml/badge.svg" /></a>
  <a href="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/govulncheck.yml"><img alt="govulncheck" src="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/govulncheck.yml/badge.svg" /></a>
  <a href="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/golangci-lint.yml"><img alt="golangci-lint" src="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/golangci-lint.yml/badge.svg" /></a>
  <a href="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/gosec.yml"><img alt="gosec" src="https://github.com/CerealKiller97/preuzmi.me/actions/workflows/gosec.yml/badge.svg" /></a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="UI" src="https://img.shields.io/badge/UI-Alpine.js%20%2B%20Tailwind-38BDF8?style=flat-square" />
  <img alt="License" src="https://img.shields.io/badge/License-AGPL--3.0--only-blue?style=flat-square" />
</p>

---

## Why

Every month: log into A1, mts, e.Sanduče… download PDFs… forget one… repeat.

**Preuzmi.me** does that for you. One refresh button (or a cron `checks` run), a clean UI for what landed, and charts for what you spent.

## Features

- **Multi-provider downloads** — A1, mts, e.Sanduče (EPS / Yettel placeholders in config)
- **Receipt dashboard** — search, filter by period / provider / paid status
- **Paid tracking** — mark receipts paid without touching `meta.json`
- **Stats** — yearly totals, monthly averages, per-provider charts
- **Notifications** — Telegram or SMTP (`off` / `per_receipt` / `all_done`)
- **Storage** — local folder or S3-compatible bucket
- **Live settings** — edit `config.json` from the UI; hot-reload without restart
- **`check_until`** — skip wasted refreshes after the day providers stop issuing bills

## Screenshots

### Receipts

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="Receipts dashboard" width="880" />
</p>

### Statistics

<p align="center">
  <img src="docs/screenshots/stats.png" alt="Statistics page" width="880" />
</p>

### Settings

<p align="center">
  <img src="docs/screenshots/settings.png" alt="Settings page" width="880" />
</p>

## Quick start

### Requirements

- Go **1.25+**
- Node **18+** (only to build CSS once)
- A `config.json` in the project root (see below)

### Install & run

```bash
git clone https://github.com/CerealKiller97/preuzmi.me.git
cd preuzmi.me

# CSS bundle (once, or after style changes)
npm install
npm run build

# Copy / edit config, then:
go run . serve
```

Open `http://localhost:5500/dashboard` (port comes from `application.port`).

Run a download pass:

```bash
go run . checks
# or click the refresh button in the header
```

## Configuration

`config.json` lives next to the binary / working directory. Secrets are never echoed back by the settings API; leave password fields empty in the UI to keep the current value.

```json
{
  "application": {
    "host": "0.0.0.0",
    "port": 5500,
    "certs": {
      "cert": "./keys/cert.pem",
      "key": "./keys/priv-key.pem"
    }
  },
  "storage": "local",
  "download_path": "./receipts",
  "check_until": 20,
  "notifications": {
    "mode": "off",
    "driver": "telegram",
    "smtp": {
      "host": "smtp.example.com",
      "port": 587,
      "username": "",
      "password": "",
      "from": "",
      "to": ""
    },
    "telegram": {
      "bot_token": "",
      "chat_id": ""
    }
  },
  "log_level": "info",
  "pretty_print": true,
  "providers": {
    "a1": { "identifier": "", "password": "" },
    "mts": { "identifier": "", "password": "" },
    "esanduce": { "identifier": "", "password": "" },
    "eps": { "identifier": "", "password": "" },
    "yettel": { "identifier": "", "password": "" }
  }
}
```

| Key | Notes |
| --- | --- |
| `storage` | `local` or `s3` |
| `download_path` | Where PDFs + `meta.json` / `payments.json` / `refresh.json` live. In Docker use an in-container path (e.g. `/data/receipts`) and bind-mount the host folder there. |
| `check_until` | Last calendar day of the month refresh is allowed (default `20`) |
| `notifications.mode` | `off` · `per_receipt` · `all_done` |
| `notifications.driver` | `telegram` or `smtp` |

Host / port / TLS certs are **frozen while the process is running** — change them in `config.json` and restart. Everything else can be applied from **Settings → Apply changes**.

## CLI

```text
preuzmi.me serve    Start the HTTP UI + API
preuzmi.me checks   Download receipts from every configured provider
preuzmi.me help     Show usage
```

`checks` is the same work as the header refresh button. Ideal for cron:

```cron
0 10 1-20 * *  cd /path/to/preuzmi.me && ./preuzmi.me checks
```

## Project layout

```text
assets/          CSS, JS, favicons, Open Graph art
docs/screenshots README UI captures
pkg/config       Load / validate / atomic save
pkg/container    DI + hot reload
pkg/http         Dashboard, stats, settings, APIs
pkg/services/*   Providers, storage, refresh, notify, payments
templates/       HTML (Alpine-driven)
```

## Open Graph

Social previews use [`assets/img/og.png`](assets/img/og.png) (1200×630). Source SVG: [`assets/img/og.svg`](assets/img/og.svg).

## License

Copyright © 2025–2026 Stefan Bogdanović  
Licensed under the terms of the **GNU Affero General Public License v3 only**.
