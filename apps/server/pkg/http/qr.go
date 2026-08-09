package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/ipsqr"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/rs/zerolog/log"
)

// ResolveIPSPayload exposes resolveIPSPayload to callers outside the HTTP layer
// (the `receipts view` CLI command), so they render the QR from the very same
// cached-then-parsed payload — and trigger the same price reconciliation — as
// the dashboard card. ok is false when the bill carries no readable QR.
func ResolveIPSPayload(ctx context.Context, store storage.Interface, rec *receipts.Repository, provider, period string) (string, bool) {
	return resolveIPSPayload(ctx, store, rec, provider, period)
}

// resolveIPSPayload returns the NBS IPS QR payload for a receipt, reading the
// database cache first and falling back to parsing the PDF on a cache miss (then
// caching the result — including an empty payload for a bill with no QR, so it
// is parsed at most once). ok is false when the bill carries no readable QR.
func resolveIPSPayload(ctx context.Context, store storage.Interface, rec *receipts.Repository, provider, period string) (string, bool) {
	payload, ok := lookupIPSPayload(ctx, store, rec, provider, period)
	if ok {
		// The QR carries the amount a banking app actually charges, so treat it as
		// the authoritative price and correct the recorded total when it drifted
		// (e.g. a provider API reporting a figure that includes an extra fee).
		reconcileIPSPrice(ctx, rec, provider, period, payload)
	}

	return payload, ok
}

// lookupIPSPayload returns the receipt's IPS payload, from the database cache
// when present and by parsing the PDF (then caching the result) on a miss. ok is
// false when the bill carries no readable QR.
func lookupIPSPayload(ctx context.Context, store storage.Interface, rec *receipts.Repository, provider, period string) (string, bool) {
	if rec != nil {
		if payload, checked, err := rec.IPSQR(ctx, provider, period); err == nil && checked {
			return payload, payload != ""
		}
	}

	// Cache miss: parse the PDF through the configured storage backend.
	key := fmt.Sprintf("%s/%s.pdf", period, provider)
	pdf, err := store.Load(ctx, key)
	if err != nil {
		// A missing PDF is an ordinary 404 for a receipt that is only a paid-only
		// stub or lives on a backend we cannot reach; nothing to extract.
		return "", false
	}

	payload, _ := ipsqr.Extract(pdf)
	if rec != nil {
		if err := rec.SetIPSQR(ctx, provider, period, payload); err != nil {
			log.Err(err).Str("provider", provider).Str("period", period).Msg("Failed to cache IPS QR payload")
		}
	}

	return payload, payload != ""
}

// reconcileIPSPrice corrects the receipt's stored price to the amount its IPS QR
// carries. It runs on every resolve but only writes when the two differ, so a
// bill filed with a wrong provider total self-heals the first time its QR is
// viewed. Best-effort: a failure is logged and swallowed.
func reconcileIPSPrice(ctx context.Context, rec *receipts.Repository, provider, period, payload string) {
	if rec == nil {
		return
	}

	amount, ok := ipsqr.Amount(payload)
	if !ok {
		return
	}

	if _, err := rec.ReconcilePrice(ctx, provider, period, amount); err != nil {
		log.Err(err).Str("provider", provider).Str("period", period).Msg("Failed to reconcile price from IPS QR")
	}
}

// receiptQRImageHandler streams a freshly rendered PNG of a receipt's IPS
// payment QR, so the dashboard card can show a crisp, scannable code. It answers
// 404 when the bill has no readable QR (e.g. a provider whose layout embeds none)
// — the card hides the QR block on that error.
func receiptQRImageHandler(store storage.Interface, receiptsStore func() *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, period, err := validate(w, r)
		if err != nil {
			return
		}

		payload, ok := resolveIPSPayload(r.Context(), store, receiptsStore(), provider, period)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		size := ipsqr.DefaultSize
		if q := r.URL.Query().Get("size"); q != "" {
			if n, convErr := strconv.Atoi(q); convErr == nil && n > 0 {
				size = n
			}
		}

		png, err := ipsqr.RenderPNG(payload, size)
		if err != nil {
			log.Err(err).Str("provider", provider).Str("period", period).Msg("Failed to render IPS QR")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		// The payload is fixed for a given bill, so let the browser cache it.
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if _, err := w.Write(png); err != nil {
			log.Err(err).Msg("Error writing QR image")
		}
	}
}

// receiptQRPayloadHandler returns the raw IPS payload string, a copy/deep-link
// fallback for a phone-only user who cannot scan their own screen. 404 when the
// bill has no QR.
func receiptQRPayloadHandler(store storage.Interface, receiptsStore func() *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, period, err := validate(w, r)
		if err != nil {
			return
		}

		payload, ok := resolveIPSPayload(r.Context(), store, receiptsStore(), provider, period)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write([]byte(payload)); err != nil {
			log.Err(err).Msg("Error writing QR payload")
		}
	}
}
