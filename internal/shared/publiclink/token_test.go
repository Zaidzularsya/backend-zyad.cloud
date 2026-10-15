package publiclink

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil || len(tok) != 43 || strings.ContainsAny(tok, "+/=") {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if !bytes.Equal(hash, HashToken(tok)) || len(hash) != 32 {
		t.Fatal("hash mismatch")
	}
	tok2, _, _ := NewToken()
	if tok == tok2 {
		t.Fatal("tokens must differ")
	}
}
