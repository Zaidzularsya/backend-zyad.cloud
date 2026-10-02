package dto

import (
	"testing"

	"zyad.cloud/internal/modules/whatsapp/domain"
)

func TestMessageFromDomainExposesDocumentName(t *testing.T) {
	doc := MessageFromDomain(domain.Message{Raw: map[string]any{"kind": "document", "filename": "QUO-1.pdf"}})
	if doc.AttachmentName != "QUO-1.pdf" {
		t.Fatalf("attachment = %q", doc.AttachmentName)
	}
	if text := MessageFromDomain(domain.Message{Raw: map[string]any{"filename": "x.pdf"}}); text.AttachmentName != "" {
		t.Fatalf("non-document attachment = %q", text.AttachmentName)
	}
}
