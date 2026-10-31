package service

import (
	"strings"
	"testing"
	"time"
)

func TestIsolatedMessage_EscapesHTMLAndListsLinks(t *testing.T) {
	due := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	invs := []TriggerInvoice{{ID: "i1", Number: "INV-001", DueDate: due}, {ID: "i2", Number: "INV-002", DueDate: due}}
	text, body, subject := IsolatedMessage("PT <T>", "<b>x</b>", invs, map[string]string{"i1": "https://pay/x?a=1&b=2"})

	if strings.Contains(body, "<b>x</b>") || !strings.Contains(body, "&lt;b&gt;x&lt;/b&gt;") {
		t.Fatalf("customer name not escaped: %s", body)
	}
	if !strings.Contains(body, "https://pay/x?a=1&amp;b=2") {
		t.Fatalf("link not escaped/listed: %s", body)
	}
	for _, s := range []string{text, body} {
		if !strings.Contains(s, "INV-001") || !strings.Contains(s, "INV-002") || !strings.Contains(s, "5 Okt 2026") {
			t.Fatalf("invoices missing: %s", s)
		}
	}
	if strings.Count(text, "https://") != 1 {
		t.Fatalf("expected exactly one link line: %s", text)
	}
	if subject == "" || !strings.Contains(subject, "PT <T>") {
		t.Fatalf("subject: %q", subject)
	}
}

func TestRestoredMessage_Plain(t *testing.T) {
	text, body, subject := RestoredMessage("", "Budi")
	if !strings.Contains(text, "Budi") || !strings.Contains(body, "Budi") || subject == "" {
		t.Fatalf("unexpected: %q %q %q", text, body, subject)
	}
	if strings.Contains(body, "<b>") {
		t.Fatalf("unexpected markup: %s", body)
	}
}
