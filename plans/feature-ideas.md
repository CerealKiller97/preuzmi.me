# Feature ideas — QR scan & payment-tracking loop

Candidate features centered on the app's core loop: download bill → scan QR → pay → mark paid → track.

Current state (for reference):
- `pkg/services/pdftext` extracts **amount + period + due date** from PDFs.
- Paid state tracked as two moments: *paid-by-you* (`paid_at`) and *confirmed-by-provider* (`confirmed_at`).
- Due dates live in `due_at`; unpaid cards show overdue/due-soon badges; optional due reminders notify.
- QR scanning is still a **manual** step the user does against the PDF; the app doesn't touch the QR yet.

---

## 1. Extract the IPS QR straight from the PDF and show it in the receipt card

Serbian utility PDFs embed an NBS IPS QR. Crop/detect it during download and render it right on the
unpaid receipt — user opens the dashboard, scans it into their banking app, taps "paid." The whole
scan→pay→track loop lives in one screen instead of hunting through PDFs. This is the app's reason to
exist, finished.

Notes:
- Extraction is **per-provider layout work** (A1, mts, EPS, e.Sanduče) — not one clean function.
- Speedup is real for a **desktop/tablet dashboard + phone banking** setup. Phone-only can't scan its
  own screen — would need a deep link / copy-to-clipboard fallback instead.

## 2. Parse iznos + poziv na broj + payee account → generate the QR yourself (fallback)

Extends the existing `pdftext` extraction. When a PDF has no embedded QR (or it's low-res), build the
IPS string from structured fields and render a clean one. Also gives real structured data instead of
just a total. Natural backbone under #1 and #3.

## 3. Due-date extraction + "unpaid & overdue" tracking + reminders ✅

The whole pitch is "no more unintentionally unpaid bills," but nothing tracks time-to-pay yet. Parse
`datum dospeća`, add an overdue badge, and fire a notification like "3 bills due in 2 days — 8,400 RSD."
This is what makes tracking actually *prevent* the miss (#1 speeds up paying; #3 makes sure you do it).

Shipped as `due_at` on receipts (PDF + e.Sanduče API), overdue/due-soon badges on the dashboard, and
`notifications.due_reminders`.

## 4. Monthly "Pay everything" view ✅

One screen: every unpaid receipt, each with its QR, a running total, and one-tap paid marking. Sit
down once a month, scan down the list, done. Turns per-receipt tracking into a batch ritual.

Shipped as **/pay** ("Plati sve") on top of #1's QR endpoints.

---

## Recommendation

- **#1 + #3** together give the biggest jump: QR lands in the UI *and* time-based tracking closes the
  "forgot one" gap.
- **#2** is the natural backbone under #1 and #3.
- Suggested first step: prototype #1 against **one provider (A1)** to see how cleanly the QR lifts out
  of the PDF before committing to all providers.
