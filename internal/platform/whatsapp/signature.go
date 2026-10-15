package whatsapp

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"strings"
)

const (
	HeaderWebhookHMAC          = "X-Webhook-Hmac"
	HeaderWebhookHMACAlgorithm = "X-Webhook-Hmac-Algorithm"
	HeaderWebhookRequestID     = "X-Webhook-Request-Id"
	HeaderWebhookTimestamp     = "X-Webhook-Timestamp"

	webhookHMACAlgorithm = "sha512"
)

// VerifyWebhookSignature checks a WAHA webhook: X-Webhook-Hmac must be the
// hex HMAC-SHA512 of the raw body with key. body must be the exact bytes
// received, before any JSON decoding.
func VerifyWebhookSignature(body []byte, headers http.Header, key string) error {
	if key == "" {
		return ErrWebhookKeyUnset
	}
	if algorithm := strings.TrimSpace(headers.Get(HeaderWebhookHMACAlgorithm)); algorithm != "" &&
		!strings.EqualFold(algorithm, webhookHMACAlgorithm) {
		return ErrInvalidSignature
	}

	received, err := hex.DecodeString(strings.TrimSpace(headers.Get(HeaderWebhookHMAC)))
	if err != nil || len(received) == 0 {
		return ErrInvalidSignature
	}

	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(body)
	if !hmac.Equal(received, mac.Sum(nil)) {
		return ErrInvalidSignature
	}
	return nil
}
