package auth

import "testing"

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "correct-password" {
		t.Fatal("expected password hash to differ from plain password")
	}
	if !VerifyPassword("correct-password", hash) {
		t.Fatal("expected password verification to pass")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("expected wrong password verification to fail")
	}
}

func TestVerifyPasswordRejectsEmptyHash(t *testing.T) {
	if VerifyPassword("password", "") {
		t.Fatal("expected empty hash to be invalid")
	}
}
