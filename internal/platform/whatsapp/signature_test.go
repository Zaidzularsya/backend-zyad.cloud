package whatsapp

import (
	"errors"
	"net/http"
	"testing"
)

// Test vector from the WAHA docs (https://waha.devlike.pro/docs/how-to/events/).
const (
	docsBody = `{"event":"message","session":"default","engine":"WEBJS"}`
	docsKey  = "my-secret-key"
	docsHMAC = "208f8a55dde9e05519e898b10b89bf0d0b3b0fdf11fdbf09b6b90476301b98d8097c462b2b17a6ce93b6b47a136cf2e78a33a63f6752c2c1631777076153fa89"
)

func signedHeaders(signature string) http.Header {
	headers := http.Header{}
	headers.Set(HeaderWebhookHMAC, signature)
	headers.Set(HeaderWebhookHMACAlgorithm, "sha512")
	return headers
}

func TestVerifyWebhookSignatureDocsVector(t *testing.T) {
	if err := VerifyWebhookSignature([]byte(docsBody), signedHeaders(docsHMAC), docsKey); err != nil {
		t.Fatalf("VerifyWebhookSignature() error = %v", err)
	}
}

func TestVerifyWebhookSignatureRejects(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		headers http.Header
		key     string
		want    error
	}{
		{"wrong key", docsBody, signedHeaders(docsHMAC), "other-key", ErrInvalidSignature},
		{"tampered body", docsBody + " ", signedHeaders(docsHMAC), docsKey, ErrInvalidSignature},
		{"missing signature", docsBody, http.Header{}, docsKey, ErrInvalidSignature},
		{"non-hex signature", docsBody, signedHeaders("not-hex"), docsKey, ErrInvalidSignature},
		{"unsupported algorithm", docsBody, func() http.Header {
			h := signedHeaders(docsHMAC)
			h.Set(HeaderWebhookHMACAlgorithm, "sha256")
			return h
		}(), docsKey, ErrInvalidSignature},
		{"key not configured", docsBody, signedHeaders(docsHMAC), "", ErrWebhookKeyUnset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyWebhookSignature([]byte(tt.body), tt.headers, tt.key)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifyWebhookSignatureWithoutAlgorithmHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set(HeaderWebhookHMAC, docsHMAC)
	if err := VerifyWebhookSignature([]byte(docsBody), headers, docsKey); err != nil {
		t.Fatalf("VerifyWebhookSignature() error = %v", err)
	}
}
