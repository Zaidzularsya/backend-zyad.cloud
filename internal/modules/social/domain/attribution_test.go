package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizersDropInvalidUTF8AndNUL(t *testing.T) {
	ua := NormalizeUserAgent("Mozilla\xff\xfe/5.0\x00 (X11)")
	if ua != "Mozilla/5.0 (X11)" || !utf8.ValidString(ua) || strings.ContainsRune(ua, 0) {
		t.Fatalf("user agent = %q", ua)
	}
	ch := NormalizeChannel(" W\u0000A\xff ")
	if ch != "wa" {
		t.Fatalf("channel = %q", ch)
	}
}

func TestNormalizersSanitizeBeforeTruncating(t *testing.T) {
	// 200 NUL bytes in front must not consume the column width.
	ua := NormalizeUserAgent(strings.Repeat("\x00", 200) + "agent")
	if ua != "agent" {
		t.Fatalf("user agent = %q", ua)
	}
	if got := NormalizeUserAgent(strings.Repeat("é", 250)); utf8.RuneCountInString(got) != maxUserAgentLength || !utf8.ValidString(got) {
		t.Fatalf("truncated user agent: %d runes", utf8.RuneCountInString(got))
	}
	if got := NormalizeChannel(strings.Repeat("\xff", 30) + "telegram"); got != "telegram" {
		t.Fatalf("channel = %q", got)
	}
}
