<p align="center">
  <img src="assets/img/og.png" alt="Preuzmi.me — automatic receipt downloads" width="880" />
</p>

<h1 align="center">Preuzmi.me</h1>

<p align="center">
  <strong>English</strong> · <a href="README.sr.md">Srpski</a>
</p>

<p align="center">
  <strong>Automatic receipt downloads</strong> for internet, phone, and electricity — in one place.<br/>
  An easy dashboard for tracking paid receipts and spending stats, so there are no more unintentionally unpaid bills.
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#features">Features</a> ·
  <a href="#screenshots">Screenshots</a> ·
  <a href="#configuration">Config</a> ·
  <a href="#cli">CLI</a> ·
  <a href="#run-from-source">From source</a>
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

## Quick start

The quickest way to run Preuzmi.me is with Docker. The image is multi-arch and distroless — the SQL schema is embedded, so only `config.json` and a downloads folder need mounting.

First create a `config.json` (see [Configuration](#configuration)) with `"download_path": "/data/receipts"`.

### docker run

```bash
docker run -d --name preuzmi \
  -p 5500:5500 \
  -v "$PWD/config.json:/app/config.json:ro" \
  -v preuzmi-receipts:/data/receipts \
  ghcr.io/cerealkiller97/preuzmi.me:1.1.0
```

### docker compose

```yaml
services:
  preuzmi:
    image: ghcr.io/cerealkiller97/preuzmi.me:1.1.0
    container_name: preuzmi.me
    ports:
      - '5500:5500'
    volumes:
      - ./config.json:/app/config.json:ro
      - preuzmi-receipts:/data/receipts # or a bind mount: ./receipts:/data/receipts
    restart: unless-stopped

volumes:
  preuzmi-receipts:
```

```bash
docker compose up -d
```

Open `http://localhost:5500/dashboard` (port comes from `application.port`).

Run a download pass (same work as the header refresh button):

```bash
docker exec preuzmi /app/preuzmi checks
```

## Features

- **Multi-provider downloads** — A1, mts, EPS and e.Sanduče sign in with each provider's own platform credentials; Yettel and eUpravnik read the invoice from your mailbox over IMAP (for now)
- **Smart refresh** — skips providers whose previous-month receipt is settled (`confirmed_at` and `paid_at` set); unpaid or unverified bills keep running so status can flip
- **Receipt dashboard** — search, filter by period / provider / paid status
- **Paid tracking** — mark receipts paid from the UI or the `receipts` CLI; tracked as two separate moments: *paid by you* and *confirmed by the provider*
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

## Configuration

`config.json` lives next to the binary / working directory. Secrets are never echoed back by the settings API; leave password fields empty in the UI to keep the current value.

```json
{
  "application": {
    "host": "0.0.0.0",
    "port": 5500
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
  "email": {
    "provider": "gmail",
    "mailbox": "INBOX"
  },
  "providers": {
    "a1": { "identifier": "", "password": "" },
    "mts": { "identifier": "", "password": "" },
    "esanduce": { "identifier": "", "password": "" },
    "eps": { "identifier": "", "password": "" },
    "yettel": { "identifier": "you@gmail.com", "password": "gmail-app-password", "mailbox": "Racuni/Yettel" },
    "eupravnik": { "identifier": "you@gmail.com", "password": "gmail-app-password", "mailbox": "Racuni/Eupravnik" }
  }
}
```

| Key                        | Notes                                                                                                                                                              |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `storage`                  | `local` or `s3`                                                                                                                                                    |
| `download_path`            | Where PDFs + `meta.json` / `receipts.db` / `refresh.json` live. In Docker use an in-container path (e.g. `/data/receipts`) and bind-mount the host folder there. |
| `check_until`              | Last calendar day of the month refresh is allowed (default `20`)                                                                                                   |
| `notifications.mode`       | `off` · `per_receipt` · `all_done`                                                                                                                                 |
| `notifications.driver`     | `telegram` or `smtp`                                                                                                                                               |
| `email.provider`           | Mailbox driver for the email-based providers (eUpravnik / Yettel). **Currently `gmail` only.**                                                                     |
| `providers.<name>.mailbox` | Gmail label to search for that provider's invoice. Empty → falls back to `email.mailbox`, then `INBOX`.                                                            |

> **Two login models.** **A1, mts, EPS and e.Sanduče** authenticate against each service's own platform, so their `identifier` / `password` are your **login for that provider**. **Yettel and eUpravnik** have no usable API, so — _for now_ — the app reads the invoice PDF from your mailbox over IMAP; their `identifier` / `password` are your **Gmail address and a Google App Password**, not the provider login. See [eUpravnik & Yettel](#eupravnik--yettel--mailbox-based-gmail-only-for-now) below.

Host / port are **frozen while the process is running** — change them in `config.json` and restart. Everything else can be applied from **Settings → Apply changes**.

### eUpravnik & Yettel — mailbox-based (Gmail only, for now)

Unlike A1 / mts / EPS / e.Sanduče, which sign in to the provider's own platform, **eUpravnik and Yettel do not use platform credentials.** Neither exposes a usable API, so the app reads the invoice PDF **straight from your mailbox over IMAP**.

- **Gmail is currently the only driver** (`email.provider: "gmail"`).
- For these two providers, `identifier` / `password` are your **Gmail address and a Google [App Password](https://support.google.com/accounts/answer/185833)** — _not_ your eUpravnik / Yettel login.
- `providers.<name>.mailbox` is the Gmail **label** to search (e.g. `Racuni/Yettel`); nested labels use `/`. Empty falls back to `email.mailbox`, then `INBOX`.

#### Faster lookups with Gmail labels

Left empty, `mailbox` searches your whole `INBOX` on every refresh — slow on a large mailbox, and likelier to match the wrong PDF. Give each email provider its own Gmail label and point `mailbox` at it, so the IMAP search scans only that label. On Gmail every label _is_ an IMAP folder, so the app can select it directly.

1. **Create the label** — Gmail → _Settings_ ⚙ → _See all settings_ → _Labels_ → _Create new label_. Use a nested name like `Racuni/Yettel` (the `/` nests it under `Racuni`).
2. **Filter incoming invoices into it** — Gmail search bar → _Show search options_ → match the invoice mail (e.g. `from:(no-reply@yettel.rs)` or a subject term) → _Create filter_ → tick _Apply the label_ and choose it. Tick _Also apply to matching conversations_ to backfill existing mail.
3. **Enable IMAP on the label** — back on the _Labels_ screen, find the label and tick _Show in IMAP_. Without this the app can't open it over IMAP.
4. **Point the config at it** — set the provider's `mailbox` to the exact label path: `"mailbox": "Racuni/Yettel"`.

Resolution order is `providers.<name>.mailbox` → `email.mailbox` → `INBOX`: a per-provider label overrides the shared `email.mailbox`, and an unset value falls back to it (then `INBOX`).

## Run from source

Prefer to run it without Docker? You'll need:

- Go **1.26+**
- Node **18+** (only to build CSS once)
- A `config.json` in the project root (see [Configuration](#configuration))

```bash
git clone https://github.com/CerealKiller97/preuzmi.me.git
cd preuzmi.me

# CSS bundle (once, or after style changes)
npm install
npm run build

# Copy / edit config, then:
go run . serve
```

Open `http://localhost:5500/dashboard`, then run a download pass:

```bash
go run . checks
# or click the refresh button in the header
```

## CLI

```text
preuzmi.me serve      Start the HTTP UI + API
preuzmi.me checks     Download receipts from every configured provider
preuzmi.me receipts   List and update receipts from the terminal
preuzmi.me help       Show usage
```

`checks` is the same work as the header refresh button. Ideal for cron (see [Scheduling](#scheduling-cron)).

### `receipts`

Browse and update receipts without opening the dashboard — it reads and writes the same receipts database, so the terminal and the UI stay in sync.

```text
preuzmi.me receipts list [MM/YYYY]        List a period's receipts (defaults to the previous month)
preuzmi.me receipts mark:as-paid   <key>  Mark a receipt paid (key is PROVIDER/PERIOD, e.g. eps/06-2026)
preuzmi.me receipts mark:as-unpaid <key>  Clear the paid mark
```

<p align="center">
  <img src="docs/screenshots/cli.svg" alt="receipts CLI" width="820" />
</p>

`list` shows a **STATUS** (as the provider reports it) plus two timestamps: **VERIFIKOVANO** — when the provider confirmed the payment — and **PLAĆENO** — when you marked it paid yourself.

> **When is a receipt considered paid?** When the **provider confirms** it — i.e. STATUS is `plaćeno` / VERIFIKOVANO is set. STATUS and VERIFIKOVANO are the same signal (VERIFIKOVANO is just the date STATUS became `plaćeno`), so they always agree. **PLAĆENO** is your own record that you've sent the payment and is independent of the provider — a receipt can be PLAĆENO but not yet VERIFIKOVANO (as with `eps` above: paid by you, awaiting the provider's confirmation).

Colour and box drawing are auto-disabled when output isn't a terminal; it respects `NO_COLOR`, and `FORCE_COLOR` forces colour when piping.

## Scheduling (cron)

**The Docker image checks for receipts automatically** — a `crond` baked into the image runs `checks` **every day at 10:00** (container local time), so bills download with no host setup. The app's `check_until` (default `20`) decides whether a given day actually downloads, so `check_until` stays the single source of truth for the window.

- **Timezone** defaults to `Europe/Belgrade` (so 10:00 and the `check_until` day are Belgrade time). Override with `TZ` — `-e TZ=Europe/Ljubljana`, or `environment: [TZ=…]` in compose.
- **Change the time** by mounting your own crontab over `/etc/crontabs/root`.
- **Run it now**, anytime: `docker exec preuzmi /app/preuzmi checks`.

Running from source instead? Schedule it from the host's crontab — `check_until` still gates the window from inside the app:

```cron
# every day at 10:00
0 10 * * *  cd /path/to/preuzmi.me && ./preuzmi.me checks
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
