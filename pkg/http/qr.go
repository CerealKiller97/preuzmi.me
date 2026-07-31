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

// resolveIPSPayload returns the NBS IPS QR payload for a receipt, reading the
// database cache first and falling back to parsing the PDF on a cache miss (then
// caching the result — including an empty payload for a bill with no QR, so it
// is parsed at most once). ok is false when the bill carries no readable QR.
func resolveIPSPayload(ctx context.Context, store storage.Interface, rec *receipts.Repository, provider, period string) (string, bool) {
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
