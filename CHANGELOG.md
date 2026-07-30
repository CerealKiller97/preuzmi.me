# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each `## [x.y.z]` section below is published verbatim as that version's GitHub
Release, so keep the entries user-facing.

## [Unreleased]

## [1.1.0] - 30.07.2026

### ✨ Added

- 🧾 `receipts` CLI to manage receipts from the terminal — no dashboard needed. `receipts list [MM/YYYY]` prints a bordered, colour-and-emoji table for a period (defaults to the previous month), and `receipts mark:as-paid` / `mark:as-unpaid` toggle a receipt's paid state. Colour respects `NO_COLOR` / `FORCE_COLOR` and turns off automatically when the output isn't a terminal.
- 🕒 Two separate paid moments per receipt, shown as their own columns: **VERIFIKOVANO** (when the provider confirmed the payment) and **PLAĆENO** (when you marked it paid yourself).

### 🔧 Changed

- 🗃️ Paid state now lives in the receipts database as the single source of truth, replacing `receipts/payments.json`. The old file is imported automatically on first run and set aside as `payments.json.migrated`, so nothing is lost — the CLI and the dashboard now always agree.

## [1.0.2] - 29.07.2026

### 🐛 Fixed

- 🧾 e.Sanduče receipts now open as valid PDFs. The provider's print endpoint returns the PDF as base64 wrapped in a JSON string, but the raw response was being saved straight to `.pdf` — so the file was never a real PDF. It's now unwrapped and base64-decoded before saving, with a clear error instead of a broken file when the provider returns an empty document.

## [1.0.1] - 28.07.2026

### ✨ Added

- ⏰ Automatic daily receipt check — the Docker image now runs `checks` on a built-in `crond` every day at 10:00 (defaults to `Europe/Belgrade`; override with `TZ`), gated by `check_until` (default 20), so bills download with no host cron.

### 🐛 Fixed

- 🔕 No more duplicate notifications on repeated runs. A download is announced only the first time a receipt is fetched, so the daily cron no longer re-notifies about bills already on record. The `checks` command now also sends paid-confirmation messages, matching the UI refresh button — both now share one notification path.
- 🧾 No false paid-confirmations for eUpravnik / Yettel. Their status comes from the *next* month's invoice, so a first run (with no previous receipt on record) no longer fires a "confirmed paid" message for a bill you never had — the confirmation now waits until the receipt exists and a later invoice settles it.
- 🕒 The "last download" time now updates after a scheduled `checks` run, not just the UI refresh button. The daily cron previously downloaded bills without recording the run, so the dashboard kept showing a stale time as if nothing had happened; a running server now also picks up a cron run without needing a restart.

## [1.0.0] - 27.07.2026

First public release — automatic receipt downloads with a dashboard, spending
stats, and notifications, so no bill goes unintentionally unpaid.

### ✨ Added

- 📥 Multi-provider receipt downloads — A1, mts, e.Sanduče, EPS, Yettel, and eUpravnik.
- 📬 Mailbox-based providers (eUpravnik, Yettel) that read the invoice PDF straight from Gmail over IMAP, with per-provider label support.
- 📄 PDF text extraction to pull amounts and payment status out of downloaded receipts.
- 🗂️ Receipt dashboard with search and filtering by period, provider, and paid status.
- ✅ Paid tracking stored separately from `meta.json`, so marking a receipt paid never touches the downloaded files.
- 📊 Statistics page — yearly totals, monthly averages, and per-provider charts.
- 🔔 Notifications over Telegram or SMTP, with `off` / `per_receipt` / `all_done` modes plus paid-confirmation messages.
- ☁️ Pluggable storage — a local folder or any S3-compatible bucket.
- ⚙️ Live settings — edit `config.json` from the UI and hot-reload without a restart; secrets are never echoed back.
- ⏳ `check_until` window to skip wasted refreshes after providers stop issuing bills.
- 🔁 One-shot `checks` CLI command (same work as the header refresh button), ideal for cron.
- 🐳 Multi-arch (amd64 + arm64), distroless Docker image published to GHCR.
- 🌐 Bilingual documentation — English and Serbian.

[Unreleased]: https://github.com/CerealKiller97/preuzmi.me/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/CerealKiller97/preuzmi.me/compare/v1.0.2...v1.1.0
[1.0.2]: https://github.com/CerealKiller97/preuzmi.me/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/CerealKiller97/preuzmi.me/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/CerealKiller97/preuzmi.me/releases/tag/v1.0.0
