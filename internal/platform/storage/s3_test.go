package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewS3ProviderRequiresConfig(t *testing.T) {
	cases := []S3Config{
		{},
		{Endpoint: "http://localhost:9000"},
		{Endpoint: "http://localhost:9000", Bucket: "b"},
		{Endpoint: "http://localhost:9000", Bucket: "b", AccessKey: "k"},
	}
	for i, cfg := range cases {
		if _, err := NewS3Provider(cfg); err == nil {
			t.Errorf("case %d: NewS3Provider should fail for incomplete config %+v", i, cfg)
		}
	}

	if _, err := NewS3Provider(S3Config{
		Endpoint:  "http://localhost:9000",
		Bucket:    "zyad-platform",
		AccessKey: "key",
		SecretKey: "secret",
	}); err != nil {
		t.Fatalf("NewS3Provider with full config: %v", err)
	}
}

// TestS3ProviderRejectsUnsafeKeys memastikan guard object key berjalan sebelum
// ada panggilan jaringan ke S3.
func TestS3ProviderRejectsUnsafeKeys(t *testing.T) {
	provider, err := NewS3Provider(S3Config{
		Endpoint:  "http://127.0.0.1:9000",
		Bucket:    "zyad-platform",
		AccessKey: "key",
		SecretKey: "secret",
	})
	if err != nil {
		t.Fatalf("NewS3Provider: %v", err)
	}
	ctx := context.Background()

	invalidKeys := []string{"", "../etc/passwd", "/organizations/org/public/x.png"}
	for _, key := range invalidKeys {
		if err := provider.Put(ctx, key, strings.NewReader("x"), "text/plain", 1); !errors.Is(err, ErrInvalidTenantObject) {
			t.Errorf("Put(%q) = %v, want ErrInvalidTenantObject", key, err)
		}
		if err := provider.Delete(ctx, key); !errors.Is(err, ErrInvalidTenantObject) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidTenantObject", key, err)
		}
		if _, err := provider.PresignGet(ctx, key); !errors.Is(err, ErrInvalidTenantObject) {
			t.Errorf("PresignGet(%q) = %v, want ErrInvalidTenantObject", key, err)
		}
	}

	// Object key valid tapi bukan kelas public harus ditolak OpenPublic tanpa
	// menyentuh jaringan.
	nonPublic := []string{
		"organizations/11111111-1111-1111-1111-111111111111/private/landing-media/secret.pdf",
		"organizations/11111111-1111-1111-1111-111111111111/temp/landing-media/x.png",
	}
	for _, key := range nonPublic {
		if _, _, err := provider.OpenPublic(ctx, key); !errors.Is(err, ErrInvalidTenantObject) {
			t.Errorf("OpenPublic(%q) = %v, want ErrInvalidTenantObject", key, err)
		}
	}
}

// TestS3ProviderImplementsInterfaces adalah guard kompilasi tambahan.
func TestS3ProviderImplementsInterfaces(t *testing.T) {
	var _ MediaStorage = (*S3Provider)(nil)
	var _ PresignedStorage = (*S3Provider)(nil)
}
