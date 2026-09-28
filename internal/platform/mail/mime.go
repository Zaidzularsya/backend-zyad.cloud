package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Attachment is one file attached to a MIMEMessage.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// MIMEMessage is a user-composed email (HTML body with a plain-text
// alternative, optional attachments), as opposed to the plain-text system
// notifications sent through Mailer.Send.
type MIMEMessage struct {
	From       netmail.Address
	To         []netmail.Address
	Cc         []netmail.Address
	Subject    string
	TextBody   string
	HTMLBody   string
	MessageID  string
	InReplyTo  string
	References []string
	Date       time.Time
	// Attachments are base64-encoded into a multipart/mixed body.
	Attachments []Attachment
}

// NewMessageID returns an RFC 5322 Message-ID ("<random@domain>").
func NewMessageID(domain string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		domain = "localhost"
	}
	return "<" + hex.EncodeToString(random) + "@" + domain + ">", nil
}

// BuildMIME renders message as a raw RFC 5322 message. Bcc recipients are
// never written to the headers; pass them to SendEnvelope only.
func BuildMIME(message MIMEMessage) ([]byte, error) {
	if len(message.To) == 0 {
		return nil, errors.New("at least one recipient is required")
	}

	date := message.Date
	if date.IsZero() {
		date = time.Now()
	}

	var buf bytes.Buffer
	writeHeader := func(name, value string) {
		buf.WriteString(name + ": " + stripNewlines(value) + "\r\n")
	}
	writeHeader("From", message.From.String())
	writeHeader("To", joinAddresses(message.To))
	if len(message.Cc) > 0 {
		writeHeader("Cc", joinAddresses(message.Cc))
	}
	writeHeader("Subject", mime.QEncoding.Encode("UTF-8", stripNewlines(message.Subject)))
	writeHeader("Date", date.Format(time.RFC1123Z))
	if message.MessageID != "" {
		writeHeader("Message-ID", message.MessageID)
	}
	if message.InReplyTo != "" {
		writeHeader("In-Reply-To", message.InReplyTo)
	}
	if len(message.References) > 0 {
		writeHeader("References", strings.Join(message.References, " "))
	}
	writeHeader("MIME-Version", "1.0")

	mixed := multipart.NewWriter(&buf)
	writeHeader("Content-Type", "multipart/mixed; boundary="+mixed.Boundary())
	buf.WriteString("\r\n")

	if err := writeAlternative(mixed, message.TextBody, message.HTMLBody); err != nil {
		return nil, err
	}
	for _, attachment := range message.Attachments {
		if err := writeAttachment(mixed, attachment); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeAlternative(parent *multipart.Writer, text, html string) error {
	var body bytes.Buffer
	alternative := multipart.NewWriter(&body)
	if err := writeQuotedPrintable(alternative, "text/plain; charset=UTF-8", text); err != nil {
		return err
	}
	if html != "" {
		if err := writeQuotedPrintable(alternative, "text/html; charset=UTF-8", html); err != nil {
			return err
		}
	}
	if err := alternative.Close(); err != nil {
		return err
	}

	part, err := parent.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=" + alternative.Boundary()},
	})
	if err != nil {
		return err
	}
	_, err = part.Write(body.Bytes())
	return err
}

func writeQuotedPrintable(writer *multipart.Writer, contentType, content string) error {
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	encoder := quotedprintable.NewWriter(part)
	if _, err := io.WriteString(encoder, content); err != nil {
		return err
	}
	return encoder.Close()
}

func writeAttachment(writer *multipart.Writer, attachment Attachment) error {
	filename := stripNewlines(strings.TrimSpace(attachment.Filename))
	if filename == "" {
		filename = "attachment"
	}
	contentType := attachment.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {mime.FormatMediaType(contentType, map[string]string{"name": filename})},
		"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": filename})},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(attachment.Content)
	// RFC 2045: base64 lines are at most 76 characters.
	for len(encoded) > 76 {
		if _, err := fmt.Fprintf(part, "%s\r\n", encoded[:76]); err != nil {
			return err
		}
		encoded = encoded[76:]
	}
	_, err = fmt.Fprintf(part, "%s\r\n", encoded)
	return err
}

func joinAddresses(addresses []netmail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return strings.Join(values, ", ")
}

// stripNewlines prevents header injection through user-supplied values.
func stripNewlines(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
