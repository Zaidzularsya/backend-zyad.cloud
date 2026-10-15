package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
)

func TestBuildMIMERoundTrip(t *testing.T) {
	raw, err := BuildMIME(MIMEMessage{
		From:       netmail.Address{Name: "Sales", Address: "sales@example.com"},
		To:         []netmail.Address{{Address: "lisa@example.com"}},
		Cc:         []netmail.Address{{Address: "boss@example.com"}},
		Subject:    "Penawaran\r\nBcc: attacker@example.com",
		TextBody:   "Halo Lisa",
		HTMLBody:   "<p>Halo <b>Lisa</b></p>",
		MessageID:  "<abc@example.com>",
		InReplyTo:  "<prev@example.com>",
		References: []string{"<prev@example.com>"},
		Attachments: []Attachment{
			{Filename: "offer.pdf", ContentType: "application/pdf", Content: bytes.Repeat([]byte("x"), 200)},
		},
	})
	if err != nil {
		t.Fatalf("BuildMIME() error = %v", err)
	}

	message, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if got := message.Header.Get("Bcc"); got != "" {
		t.Fatalf("Bcc header injected: %q", got)
	}
	if got := message.Header.Get("Cc"); got != "<boss@example.com>" {
		t.Errorf("Cc = %q", got)
	}
	if got := message.Header.Get("In-Reply-To"); got != "<prev@example.com>" {
		t.Errorf("In-Reply-To = %q", got)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if strings.ContainsAny(subject, "\r\n") {
		t.Errorf("subject keeps newline: %q", subject)
	}

	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type = %q, %v", mediaType, err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])

	first, err := reader.NextPart()
	if err != nil || !strings.HasPrefix(first.Header.Get("Content-Type"), "multipart/alternative") {
		t.Fatalf("first part = %v, %v; want multipart/alternative", first.Header, err)
	}

	attachment, err := reader.NextPart()
	if err != nil {
		t.Fatalf("attachment part error = %v", err)
	}
	if attachment.FileName() != "offer.pdf" {
		t.Errorf("filename = %q", attachment.FileName())
	}
	encoded, _ := io.ReadAll(attachment)
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(encoded), "\r\n", ""))
	if err != nil || len(decoded) != 200 {
		t.Fatalf("attachment decoded %d bytes, %v; want 200", len(decoded), err)
	}
}

func TestBuildMIMERequiresRecipient(t *testing.T) {
	if _, err := BuildMIME(MIMEMessage{From: netmail.Address{Address: "a@example.com"}}); err == nil {
		t.Fatal("BuildMIME() without recipients error = nil")
	}
}
