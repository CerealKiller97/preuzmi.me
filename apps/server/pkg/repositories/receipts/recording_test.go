package receipts

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

// fakeStorage records the last Save so the test can assert the inner backend
// still ran even when indexing is skipped.
type fakeStorage struct {
	savedKey  string
	savedData []byte
}

func (f *fakeStorage) Save(_ context.Context, key string, data []byte) error {
	f.savedKey = key
	f.savedData = data

	return nil
}

func (f *fakeStorage) Load(_ context.Context, _ string) ([]byte, error) {
	return f.savedData, nil
}

// A nil store is the documented pass-through mode (the database failed to open).
// Save must still persist the PDF and must not panic on the nil *Repository.
func TestRecordingStorageNilStorePassesThrough(t *testing.T) {
	inner := &fakeStorage{}
	rec := NewRecordingStorage(inner, nil, zerolog.Nop())

	data := []byte("pdf-bytes")
	if err := rec.Save(context.Background(), "06-2026/eps.pdf", data); err != nil {
		t.Fatalf("Save with nil store: %v", err)
	}

	if inner.savedKey != "06-2026/eps.pdf" {
		t.Fatalf("inner storage not called: key = %q", inner.savedKey)
	}
	if string(inner.savedData) != string(data) {
		t.Fatalf("inner storage got wrong data: %q", inner.savedData)
	}
}
