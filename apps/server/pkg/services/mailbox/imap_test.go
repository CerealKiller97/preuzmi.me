package mailbox

import (
	"strings"
	"testing"
)

// rawWithPDF is a minimal multipart/mixed message: a text body plus a PDF
// attachment whose bytes are "%PDF-1.4\n" base64-encoded (JVBERi0xLjQK).
const rawWithPDF = "From: eUpravnik <noreply@eupravnik.rs>\r\n" +
	"Subject: Racun\r\n" +
	"Content-Type: multipart/mixed; boundary=sep\r\n" +
	"\r\n" +
	"--sep\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Vas racun je u prilogu.\r\n" +
	"--sep\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=\"racun.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"JVBERi0xLjQK\r\n" +
	"--sep--\r\n"

// rawNoAttachment is a plain text message with no attachments.
const rawNoAttachment = "From: someone@example.com\r\n" +
	"Subject: Hi\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Just a note.\r\n"

func TestReadAttachmentsExtractsPDF(t *testing.T) {
	atts, err := readAttachments(strings.NewReader(rawWithPDF))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}

	a := atts[0]
	if a.Filename != "racun.pdf" {
		t.Errorf("filename = %q, want racun.pdf", a.Filename)
	}
	if a.ContentType != "application/pdf" {
		t.Errorf("content type = %q, want application/pdf", a.ContentType)
	}
	if got := string(a.Data); got != "%PDF-1.4\n" {
		t.Errorf("data = %q, want %%PDF-1.4 header", got)
	}
}

func TestReadAttachmentsNoneWhenPlainText(t *testing.T) {
	atts, err := readAttachments(strings.NewReader(rawNoAttachment))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 0 {
		t.Fatalf("got %d attachments, want 0", len(atts))
	}
}
