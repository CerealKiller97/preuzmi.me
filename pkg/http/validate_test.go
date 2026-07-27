package http

import (
	"net/http/httptest"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
)

// TestValidateAcceptsEveryImplementedProvider guards against the receipt route
// drifting out of step with the providers that can actually produce a receipt.
// A hardcoded allowlist here once 400'd eupravnik even though its PDF existed.
func TestValidateAcceptsEveryImplementedProvider(t *testing.T) {
	for _, name := range []string{"mts", "a1", "esanduce", "eps", "yettel", "eupravnik"} {
		if !container.IsImplemented(name) {
			t.Fatalf("test fixture out of date: %q is no longer implemented", name)
		}

		req := httptest.NewRequest("GET", "/receipt/07-2026/"+name, nil)
		req.SetPathValue("provider", name)
		req.SetPathValue("period", "07-2026")
		rec := httptest.NewRecorder()

		provider, period, err := validate(rec, req)
		if err != nil {
			t.Errorf("validate rejected implemented provider %q: %v", name, err)
			continue
		}
		if provider != name || period != "07-2026" {
			t.Errorf("validate returned (%q, %q), want (%q, 07-2026)", provider, period, name)
		}
	}
}

func TestValidateRejectsUnknownProvider(t *testing.T) {
	req := httptest.NewRequest("GET", "/receipt/07-2026/bogus", nil)
	req.SetPathValue("provider", "bogus")
	req.SetPathValue("period", "07-2026")
	rec := httptest.NewRecorder()

	if _, _, err := validate(rec, req); err == nil {
		t.Fatal("expected validate to reject an unknown provider")
	}
	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
