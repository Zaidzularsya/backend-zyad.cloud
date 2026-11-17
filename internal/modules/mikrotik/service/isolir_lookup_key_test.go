package service

import "testing"

func TestLookupKey(t *testing.T) {
	same := []string{"0812 3456 789", "0812-3456-789", "+62812 3456 789", "62812-3456-789", " (0812) 3456789 "}
	want := LookupKey(same[0])
	if want == "" {
		t.Fatal("empty key")
	}
	for _, q := range same {
		if got := LookupKey(q); got != want {
			t.Errorf("LookupKey(%q)=%q want %q", q, got, want)
		}
	}
	if LookupKey("  Budi  ") != "budi" || LookupKey("BUDI") != "budi" {
		t.Errorf("username key must be trimmed lower-case: %q", LookupKey("  Budi  "))
	}
	// digits mixed with letters is a username, not a phone
	if got := LookupKey("0812abc3456"); got != "0812abc3456" {
		t.Errorf("got %q", got)
	}
	if LookupKey("budi") == want {
		t.Error("username shares key with phone")
	}
}
