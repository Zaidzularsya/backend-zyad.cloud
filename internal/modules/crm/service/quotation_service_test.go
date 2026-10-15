package service

import "testing"

func TestFormatDocumentNumber(t *testing.T) {
	got := formatDocumentNumber("QUO", 2026, 1)
	want := "QUO-2026-0001"
	if got != want {
		t.Fatalf("formatDocumentNumber() = %q, want %q", got, want)
	}

	got = formatDocumentNumber("INV", 2026, 12345)
	want = "INV-2026-12345"
	if got != want {
		t.Fatalf("formatDocumentNumber() = %q, want %q", got, want)
	}
}
