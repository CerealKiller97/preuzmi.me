# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each `## [x.y.z]` section below is published verbatim as that version's GitHub
Release, so keep the entries user-facing.

## [Unreleased]

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

[Unreleased]: https://github.com/CerealKiller97/preuzmi.me/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/CerealKiller97/preuzmi.me/releases/tag/v1.0.0
