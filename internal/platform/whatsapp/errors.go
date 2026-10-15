package whatsapp

import (
	"errors"
	"fmt"
)

var (
	ErrNotConfigured    = errors.New("whatsapp: waha client is not configured")
	ErrSessionNotFound  = errors.New("whatsapp: waha session not found")
	ErrUnauthorized     = errors.New("whatsapp: waha rejected api key")
	ErrTimeout          = errors.New("whatsapp: waha request timed out")
	ErrUpstream         = errors.New("whatsapp: waha upstream error")
	ErrInvalidSignature = errors.New("whatsapp: invalid webhook signature")
	ErrWebhookKeyUnset  = errors.New("whatsapp: webhook hmac key is not configured")
)

// APIError is a non-2xx WAHA response. It unwraps to one of the sentinels
// above so callers can use errors.Is without inspecting status codes.
// Body is a truncated response preview; WAHA error bodies never echo the
// X-Api-Key header, so it is safe to log.
type APIError struct {
	Operation  string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("waha %s failed (status %d): %s", e.Operation, e.StatusCode, e.Body)
}

func (e *APIError) Unwrap() error {
	switch {
	case e.StatusCode == 404:
		return ErrSessionNotFound
	case e.StatusCode == 401 || e.StatusCode == 403:
		return ErrUnauthorized
	default:
		return ErrUpstream
	}
}
