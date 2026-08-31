//go:build integration

package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// s3TestConfig membaca kredensial MinIO dari env. Test di-skip bila belum ada.
//
// Contoh (MinIO lokal):
//
//	STORAGE_S3_ENDPOINT=http://127.0.0.1:9000 \
//	STORAGE_S3_BUCKET=zyad-platform-test \
//	STORAGE_S3_ACCESS_KEY=minioadmin \
//	STORAGE_S3_SECRET_KEY=minioadmin \
//	go test -tags integration ./internal/platform/storage/...
func s3TestConfig(t *testing.T) S3Config {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("STORAGE_S3_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("STORAGE_S3_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("STORAGE_S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("STORAGE_S3_SECRET_KEY"))
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("STORAGE_S3_* env not set; skipping S3 integration test")
	}
	return S3Config{
		Endpoint:      endpoint,
		Region:        "us-east-1",
		Bucket:        bucket,
		AccessKey:     accessKey,
		SecretKey:     secretKey,
		UsePathStyle:  true,
		PresignExpiry: 5 * time.Minute,
	}
}

func TestS3ProviderRoundTripIntegration(t *testing.T) {
	provider, err := NewS3Provider(s3TestConfig(t))
	if err != nil {
		t.Fatalf("NewS3Provider: %v", err)
	}
	ctx := context.Background()
	key := "organizations/11111111-1111-1111-1111-111111111111/public/landing-media/it-roundtrip.txt"
	payload := "s3-integration-payload"

	if err := provider.Put(ctx, key, strings.NewReader(payload), "text/plain", int64(len(payload))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = provider.Delete(ctx, key) })

	body, contentType, err := provider.OpenPublic(ctx, key)
	if err != nil {
		t.Fatalf("OpenPublic: %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if string(got) != payload {
		t.Fatalf("OpenPublic content = %q, want %q", got, payload)
	}
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("OpenPublic contentType = %q, want text/plain*", contentType)
	}

	if err := provider.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := provider.Delete(ctx, key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Delete after delete = %v, want ErrObjectNotFound", err)
	}
}

func TestS3ProviderPresignIntegration(t *testing.T) {
	provider, err := NewS3Provider(s3TestConfig(t))
	if err != nil {
		t.Fatalf("NewS3Provider: %v", err)
	}
	ctx := context.Background()
	key := "organizations/11111111-1111-1111-1111-111111111111/public/landing-media/it-presign.txt"
	payload := "presigned-upload-body"
	t.Cleanup(func() { _ = provider.Delete(ctx, key) })

	putRes, err := provider.PresignPut(ctx, key, "text/plain", int64(len(payload)))
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, putRes.Method, putRes.URL, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build PUT request: %v", err)
	}
	for k, v := range putRes.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("presigned PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned PUT status = %d, want 200", resp.StatusCode)
	}

	getRes, err := provider.PresignGet(ctx, key)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	getReq, _ := http.NewRequestWithContext(ctx, getRes.Method, getRes.URL, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("presigned GET: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("presigned GET status = %d, want 200", getResp.StatusCode)
	}
	got, _ := io.ReadAll(getResp.Body)
	if string(got) != payload {
		t.Fatalf("presigned GET body = %q, want %q", got, payload)
	}
}
