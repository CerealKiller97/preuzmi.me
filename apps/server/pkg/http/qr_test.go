package http

import (
	"bytes"
	"context"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
)

// sampleBill copies one of the ipsqr package's real sample bills into dir under
// the storage key the handler will look up.
func sampleBill(t *testing.T, dir, period, provider, sample string) {
	t.Helper()

	src := filepath.Join("..", "services", "ipsqr", "testdata", sample)
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		t.Skipf("fixture %s not present (gitignored real bill); skipping", sample)
	}
	if err != nil {
		t.Fatalf("read sample %s: %v", sample, err)
	}

	dst := filepath.Join(dir, period, provider+".pdf")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func qrRequest(period, provider string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/receipt/"+period+"/"+provider+"/qr.png", nil)
	r.SetPathValue("period", period)
	r.SetPathValue("provider", provider)
	return r
}

// TestReceiptQRImageHandlerServesAndCaches verifies that a bill with an embedded
// IPS QR renders a decodable PNG and that the payload is cached in the database
// so a second request does not need to re-parse the PDF.
func TestReceiptQRImageHandlerServesAndCaches(t *testing.T) {
	dir := t.TempDir()
	sampleBill(t, dir, "05-2026", "mts", "mts.pdf")

	store := storage.NewLocal(dir)
	rec, err := receipts.New(dir)
	if err != nil {
		t.Fatalf("receipts.New: %v", err)
	}
	defer rec.Close() //nolint:errcheck

	if err := rec.Record(context.Background(), receipts.Receipt{
		Provider: "mts", Period: "05-2026", StorageKey: "05-2026/mts.pdf", SizeBytes: 1,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	handler := receiptQRImageHandler(store, func() *receipts.Repository { return rec })

	rr := httptest.NewRecorder()
	handler(rr, qrRequest("05-2026", "mts"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
	if _, _, err := image.Decode(bytes.NewReader(rr.Body.Bytes())); err != nil {
		t.Fatalf("response body is not a decodable PNG: %v", err)
	}

	// The payload must now be cached (ips_checked = 1, non-empty payload).
	payload, checked, err := rec.IPSQR(context.Background(), "mts", "05-2026")
	if err != nil {
		t.Fatalf("IPSQR: %v", err)
	}
	if !checked || payload == "" {
		t.Fatalf("expected cached payload after render, got checked=%v payload=%q", checked, payload)
	}

	// A second request served from cache still returns a PNG.
	rr2 := httptest.NewRecorder()
	handler(rr2, qrRequest("05-2026", "mts"))
	if rr2.Code != http.StatusOK {
		t.Fatalf("cached request status = %d, want 200", rr2.Code)
	}
}

// TestReceiptQRImageHandlerNoQR verifies a bill with no readable IPS QR answers
// 404 (so the card hides the QR block) and caches the negative result.
func TestReceiptQRImageHandlerNoQR(t *testing.T) {
	dir := t.TempDir()

	// Minimal PDF with no images / QR — not a real bill fixture.
	period, provider := "06-2026", "a1"
	dst := filepath.Join(dir, period, provider+".pdf")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	minimal := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
	if err := os.WriteFile(dst, minimal, 0o644); err != nil {
		t.Fatal(err)
	}

	store := storage.NewLocal(dir)
	rec, err := receipts.New(dir)
	if err != nil {
		t.Fatalf("receipts.New: %v", err)
	}
	defer rec.Close() //nolint:errcheck

	if err := rec.Record(context.Background(), receipts.Receipt{
		Provider: provider, Period: period, StorageKey: period + "/" + provider + ".pdf", SizeBytes: 1,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	handler := receiptQRImageHandler(store, func() *receipts.Repository { return rec })

	rr := httptest.NewRecorder()
	handler(rr, qrRequest(period, provider))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a bill with no QR", rr.Code)
	}

	// The negative result is cached: checked, empty payload.
	payload, checked, err := rec.IPSQR(context.Background(), provider, period)
	if err != nil {
		t.Fatalf("IPSQR: %v", err)
	}
	if !checked || payload != "" {
		t.Fatalf("expected cached empty payload, got checked=%v payload=%q", checked, payload)
	}
}
