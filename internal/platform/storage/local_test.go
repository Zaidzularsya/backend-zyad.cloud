package storage

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLocalProviderPutResolveDelete(t *testing.T) {
	provider, err := NewLocalProvider(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}
	ctx := context.Background()
	key := "organizations/11111111-1111-1111-1111-111111111111/public/landing-media/abc123.png"

	if err := provider.Put(ctx, key, strings.NewReader("image-bytes"), "image/png", 11); err != nil {
		t.Fatalf("Put: %v", err)
	}
	path, err := provider.ResolvePublic(key)
	if err != nil {
		t.Fatalf("ResolvePublic: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "image-bytes" {
		t.Fatalf("stored content = %q, err = %v", content, err)
	}
	if err := provider.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := provider.ResolvePublic(key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("ResolvePublic after delete = %v, want ErrObjectNotFound", err)
	}
}

func TestLocalProviderRejectsUnsafeKeys(t *testing.T) {
	provider, err := NewLocalProvider(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}
	ctx := context.Background()
	privateKey := "organizations/11111111-1111-1111-1111-111111111111/private/landing-media/secret.pdf"
	if err := provider.Put(ctx, privateKey, strings.NewReader("private"), "application/pdf", 7); err != nil {
		t.Fatalf("Put private: %v", err)
	}

	cases := []string{
		"../etc/passwd",
		"/organizations/org/public/x.png",
		"organizations/org/temp/x.png",
		privateKey,
		"",
	}
	for _, key := range cases {
		if _, err := provider.ResolvePublic(key); err == nil {
			t.Errorf("ResolvePublic(%q) should be rejected", key)
		}
	}
}
