<p align="center">
  <img src="assets/img/og.png" alt="Preuzmi.me — automatsko preuzimanje računa" width="880" />
</p>

<h1 align="center">Preuzmi.me</h1>

<p align="center">
  <a href="README.md">English</a> · <strong>Srpski</strong>
</p>

<p align="center">
  <strong>Automatsko preuzimanje računa</strong> za internet, telefon i struju — na jednom mestu.<br/>
  Dashboard, statistika troškova, obaveštenja i podešavanja koja se primenjuju bez restarta.
</p>

<p align="center">
  <a href="#mogucnosti">Mogućnosti</a> ·
  <a href="#snimci-ekrana">Snimci ekrana</a> ·
  <a href="#brzi-start">Brzi start</a> ·
  <a href="#konfiguracija">Konfiguracija</a> ·
  <a href="#cli">CLI</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="UI" src="https://img.shields.io/badge/UI-Alpine.js%20%2B%20Tailwind-38BDF8?style=flat-square" />
  <img alt="License" src="https://img.shields.io/badge/License-AGPL--3.0--only-blue?style=flat-square" />
</p>

---

## Zašto

Svaki mesec: uloguj se na A1, mts, e.Sanduče… preuzmi PDF… zaboravi jedan… ponovi.

**Preuzmi.me** to radi umesto tebe. Jedno dugme za osvežavanje (ili cron `checks`), pregled šta je stiglo i grafikoni koliko si potrošio.

## Mogućnosti

- **Više provajdera** — A1, mts, e.Sanduče (EPS / Yettel kao mesta u configu)
- **Dashboard računa** — pretraga, filter po periodu / provajderu / statusu plaćanja
- **Praćenje plaćanja** — označi račun kao plaćen bez diranja `meta.json`
- **Statistika** — godišnji zbir, mesečni prosek, grafikoni po provajderu
- **Obaveštenja** — Telegram ili SMTP (`off` / `per_receipt` / `all_done`)
- **Skladište** — lokalni folder ili S3-kompatibilni bucket
- **Živa podešavanja** — izmena `config.json` iz UI-ja; hot-reload bez restarta
- **`check_until`** — bez bespotrebnog osvežavanja posle dana kada provajderi više ne izdaju račune

## Snimci ekrana

### Računi

<p align="center">
  <img src="docs/screenshots/dashboard.png" alt="Dashboard računa" width="880" />
</p>

### Statistika

<p align="center">
  <img src="docs/screenshots/stats.png" alt="Stranica statistike" width="880" />
</p>

### Podešavanja

<p align="center">
  <img src="docs/screenshots/settings.png" alt="Stranica podešavanja" width="880" />
</p>

## Brzi start

### Zahtevi

- Go **1.25+**
- Node **18+** (samo za jednokratni build CSS-a)
- `config.json` u korenu projekta (vidi ispod)

### Instalacija i pokretanje

```bash
git clone https://github.com/CerealKiller97/preuzmi.me.git
cd preuzmi.me

# CSS bundle (jednom, ili posle izmene stilova)
npm install
npm run build

# Kopiraj / izmeni config, zatim:
go run . serve
```

Otvori `http://localhost:5500/dashboard` (port je `application.port`).

Pokreni preuzimanje:

```bash
go run . checks
# ili klikni dugme za osvežavanje u headeru
```

## Konfiguracija

`config.json` stoji pored binarnog fajla / u radnom direktorijumu. Tajne vrednosti API nikad ne vraća; u UI ostavi polja za lozinku prazna da zadržiš postojeću vrednost.

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

| Ključ | Napomena |
| --- | --- |
| `storage` | `local` ili `s3` |
| `download_path` | Gde idu PDF-ovi + `meta.json` / `payments.json` / `refresh.json`. U Dockeru koristi putanju unutar kontejnera (npr. `/data/receipts`) i mapiraj host folder tamo. |
| `check_until` | Poslednji dan u mesecu kada je osvežavanje dozvoljeno (podrazumevano `20`) |
| `notifications.mode` | `off` · `per_receipt` · `all_done` |
| `notifications.driver` | `telegram` ili `smtp` |

Host / port / TLS sertifikati su **zaključani dok proces radi** — menjaju se u `config.json` + restart. Sve ostalo ide preko **Podešavanja → Primeni promene**.

## CLI

```text
preuzmi.me serve    Pokreće HTTP UI + API
preuzmi.me checks   Preuzima račune sa svih podešenih provajdera
preuzmi.me help     Prikazuje pomoć
```

`checks` radi isto što i dugme za osvežavanje u headeru. Pogodno za cron:

```cron
0 10 1-20 * *  cd /putanja/do/preuzmi.me && ./preuzmi.me checks
```

## Struktura projekta

```text
assets/          CSS, JS, favikone, Open Graph
docs/screenshots snimci ekrana za README
pkg/config       učitavanje / validacija / atomski save
pkg/container    DI + hot reload
pkg/http         dashboard, statistika, podešavanja, API
pkg/services/*   provajderi, skladište, refresh, notify, payments
templates/       HTML (Alpine)
```

## Open Graph

Za deljenje linkova koristi se [`assets/img/og.png`](assets/img/og.png) (1200×630). Izvor: [`assets/img/og.svg`](assets/img/og.svg).

## Licenca

Copyright © 2025–2026 Stefan Bogdanović  
Licencirano pod uslovima **GNU Affero General Public License v3 only**.
