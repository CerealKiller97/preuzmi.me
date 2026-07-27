package yettel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/mailbox"
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
}

func (m *memStorage) Save(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.key, m.data = key, data
	return nil
}

func (m *memStorage) Load(_ context.Context, _ string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data, nil
}

func pdfAttachment(name string) mailbox.Attachment {
	return mailbox.Attachment{Filename: name, ContentType: "application/pdf", Data: []byte("%PDF-1.4\n")}
}

func TestDownloadReceiptSavesLatestPDF(t *testing.T) {
	older := mailbox.Message{
		From:        "eracun@yettel.rs",
		Date:        time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
		Attachments: []mailbox.Attachment{pdfAttachment("maj.pdf")},
	}
	newer := mailbox.Message{
		From:        "eracun@yettel.rs",
		Date:        time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
		Attachments: []mailbox.Attachment{pdfAttachment("jun.pdf")},
	}

	reader := &fakeReader{messages: []mailbox.Message{older, newer}}
	store := &memStorage{}

	s := New(reader, zerolog.Nop(), store, nil)
	if err := s.DownloadReceipt(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The stub PDF bytes aren't a real PDF, so parsing fails and the receipt is
	// filed under the newer email's date.
	if store.key != "06-2026/yettel.pdf" {
		t.Errorf("saved key = %q, want 06-2026/yettel.pdf", store.key)
	}
	if reader.lastCrit.From != senderFilter {
		t.Errorf("searched From = %q, want %q", reader.lastCrit.From, senderFilter)
	}
}

func TestDownloadReceiptErrorsWithoutPDF(t *testing.T) {
	msg := mailbox.Message{
		From:        "eracun@yettel.rs",
		Date:        time.Now(),
		Attachments: []mailbox.Attachment{{Filename: "note.txt", ContentType: "text/plain", Data: []byte("hi")}},
	}
	reader := &fakeReader{messages: []mailbox.Message{msg}}
	store := &memStorage{}

	s := New(reader, zerolog.Nop(), store, nil)
	if err := s.DownloadReceipt(); err == nil {
		t.Fatal("expected an error when no PDF attachment is present")
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

func TestSearchSinceIsStartOfPreviousMonth(t *testing.T) {
	got := searchSince(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	want := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("searchSince = %v, want %v", got, want)
	}
}

func TestInvoiceFromRows(t *testing.T) {
	// Rows as reconstructed from the real Yettel invoice layout.
	rows := []string{
		"NEMANJA MITIĆJUN 2026Obračunski period: 01.06.2026. - 30.06.2026.",
		"Zaduženje za JUN 2026:3.001,01",
		"poziv na broj 92-21533646-2606. ...Prethodno stanje:0,00",
		"UKUPNO ZA PLAĆANJE3.001,01",
	}

	in, err := invoiceFromRows(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !in.hasTotal || in.total != 3001.01 {
		t.Errorf("total = %v (has=%v), want 3001.01", in.total, in.hasTotal)
	}
	if !in.hasPeriod || in.periodString() != "06-2026" {
		t.Errorf("period = %s (has=%v), want 06-2026", in.periodString(), in.hasPeriod)
	}
	if !in.hasPrevBalance || in.prevBalance != 0 {
		t.Errorf("prevBalance = %v (has=%v), want 0", in.prevBalance, in.hasPrevBalance)
	}
	if !in.previousSettled() {
		t.Error("previousSettled() = false, want true for Prethodno stanje 0,00")
	}
	if in.previousPeriodString() != "05-2026" {
		t.Errorf("previous = %s, want 05-2026", in.previousPeriodString())
	}
}

func TestInvoiceFromRowsPreviousNotSettled(t *testing.T) {
	in, err := invoiceFromRows([]string{
		"Obračunski period: 01.06.2026. - 30.06.2026.",
		"UKUPNO ZA PLAĆANJE4.500,00",
		"Prethodno stanje:1.499,00",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.total != 4500.00 {
		t.Errorf("total = %v, want 4500.00", in.total)
	}
	if in.previousSettled() {
		t.Error("previousSettled() = true, want false when a balance is carried over")
	}
}

func TestInvoiceFromRowsErrorsWhenEmpty(t *testing.T) {
	if _, err := invoiceFromRows([]string{"Hvala što ste odabrali Yettel"}); err == nil {
		t.Error("expected an error when no total or period is found")
	}
}
