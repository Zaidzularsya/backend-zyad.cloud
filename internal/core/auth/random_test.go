package auth

import "testing"

func TestNewRandomToken(t *testing.T) {
	token, err := NewRandomToken(32)
	if err != nil {
		t.Fatalf("new random token: %v", err)
	}
	if token == "" {
		t.Fatal("expected random token")
	}
}

func TestNewRandomTokenRejectsInvalidSize(t *testing.T) {
	if _, err := NewRandomToken(0); err == nil {
		t.Fatal("expected invalid token size error")
	}
}

func TestHashToken(t *testing.T) {
	hash := HashToken("plain-token")
	if hash == "" {
		t.Fatal("expected token hash")
	}
	if hash == "plain-token" {
		t.Fatal("expected token hash to differ from plain token")
	}
	if hash != HashToken("plain-token") {
		t.Fatal("expected deterministic token hash")
	}
}
