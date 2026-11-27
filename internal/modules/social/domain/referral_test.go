package domain

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestNewReferralCodeAlphabetAndLength(t *testing.T) {
	for i := 0; i < 500; i++ {
		code, err := NewReferralCode(rand.Reader)
		if err != nil {
			t.Fatalf("NewReferralCode: %v", err)
		}
		if len(code) != ReferralCodeLength {
			t.Fatalf("code %q has length %d, want %d", code, len(code), ReferralCodeLength)
		}
		for _, r := range code {
			if !strings.ContainsRune(ReferralAlphabet, r) {
				t.Fatalf("code %q contains %q outside the alphabet", code, r)
			}
		}
		if !IsReferralCode(code) {
			t.Fatalf("IsReferralCode(%q) = false", code)
		}
	}

	// The alphabet has 32 symbols, so each random byte maps by its low 5 bits.
	code, err := NewReferralCode(bytes.NewReader([]byte{0, 1, 2, 7, 8, 32, 254, 255}))
	if err != nil {
		t.Fatalf("NewReferralCode: %v", err)
	}
	if code != "2349A2YZ" {
		t.Fatalf("code = %q", code)
	}

	if _, err := NewReferralCode(bytes.NewReader([]byte{1, 2})); err == nil {
		t.Fatal("short random source must return an error")
	}
}

func TestNormalizeReferralCode(t *testing.T) {
	if got := NormalizeReferralCode("  abcd2345 "); got != "ABCD2345" {
		t.Fatalf("NormalizeReferralCode = %q", got)
	}
	for _, bad := range []string{"", "ABCD234", "ABCD23456", "ABCD2341", "ABCD234O", "ABCD234-"} {
		if IsReferralCode(bad) {
			t.Fatalf("IsReferralCode(%q) = true", bad)
		}
	}
}
