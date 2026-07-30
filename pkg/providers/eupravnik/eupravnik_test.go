package eupravnik

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/mailbox"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/rs/zerolog"
)

// fakeReader is an in-memory mailbox.Reader for tests.
type fakeReader struct {
	messages []mailbox.Message
	err      error
	lastCrit mailbox.Criteria
}

func (f *fakeReader) Search(_ context.Context, c mailbox.Criteria) ([]mailbox.Message, error) {
	f.lastCrit = c
	return f.messages, f.err
}

// memStorage records the last saved key/data.
type memStorage struct {
	mu   sync.Mutex
	key  string
	data []byte
	err  error
}

func (m *memStorage) Save(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.key, m.data = key, data
	return nil
}

func (m *memStorage) Load(_ context.Context, _ string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

func pdfAttachment(name string) mailbox.Attachment {
	return mailbox.Attachment{Filename: name, ContentType: "application/pdf", Data: []byte("%PDF-1.4\n")}
}

func TestDownloadReceiptSavesLatestPDF(t *testing.T) {
	older := mailbox.Message{
		From:        "noreply@eupravnik.rs",
		Date:        time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
		Attachments: []mailbox.Attachment{pdfAttachment("maj.pdf")},
	}
	newer := mailbox.Message{
		From:        "noreply@eupravnik.rs",
		Date:        time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		Attachments: []mailbox.Attachment{pdfAttachment("jun.pdf")},
	}

	reader := &fakeReader{messages: []mailbox.Message{older, newer}}
	store := &memStorage{}

	s := New(reader, zerolog.Nop(), store, nil)
	if err := s.DownloadReceipt(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.key != "06-2026/eupravnik.pdf" {
		t.Errorf("saved key = %q, want 06-2026/eupravnik.pdf", store.key)
	}
	if string(store.data) != "%PDF-1.4\n" {
		t.Errorf("saved data = %q, want the PDF bytes", store.data)
	}
	if reader.lastCrit.From != senderFilter {
		t.Errorf("searched From = %q, want %q", reader.lastCrit.From, senderFilter)
	}
}

func TestDownloadReceiptNoPDFIsNoOp(t *testing.T) {
	msg := mailbox.Message{
		From:        "noreply@eupravnik.rs",
		Date:        time.Now(),
		Attachments: []mailbox.Attachment{{Filename: "note.txt", ContentType: "text/plain", Data: []byte("hi")}},
	}
	reader := &fakeReader{messages: []mailbox.Message{msg}}
	store := &memStorage{}

	// No invoice PDF in the mailbox is a normal "nothing to fetch yet", reported
	// as the ErrNoReceipt sentinel (not a failure), and nothing is saved.
	s := New(reader, zerolog.Nop(), store, nil)
	if err := s.DownloadReceipt(); !errors.Is(err, provider.ErrNoReceipt) {
		t.Fatalf("no PDF should report ErrNoReceipt, got: %v", err)
	}
	if store.key != "" {
		t.Errorf("nothing should have been saved, got key %q", store.key)
	}
}

func TestDownloadReceiptPropagatesSearchError(t *testing.T) {
	reader := &fakeReader{err: errors.New("imap down")}
	s := New(reader, zerolog.Nop(), &memStorage{}, nil)
	if err := s.DownloadReceipt(); err == nil {
		t.Fatal("expected the search error to propagate")
	}
}

func TestFirstPDFMatchesByFilenameWhenContentTypeWrong(t *testing.T) {
	atts := []mailbox.Attachment{
		{Filename: "racun.pdf", ContentType: "application/octet-stream", Data: []byte("x")},
	}
	if _, ok := firstPDF(atts); !ok {
		t.Error("expected a .pdf filename to match even with a generic content type")
	}
}

func TestPeriodFromDate(t *testing.T) {
	got := periodFromDate(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC))
	if got != "06-2026" {
		t.Errorf("periodFromDate = %q, want 06-2026", got)
	}
	if got := periodFromDate(time.Time{}); len(got) < 6 {
		t.Errorf("periodFromDate zero fallback = %q, want a non-empty MM-YYYY", got)
	}
}

func TestSearchSinceIsStartOfPreviousMonth(t *testing.T) {
	got := searchSince(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	want := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("searchSince = %v, want %v", got, want)
	}
}
