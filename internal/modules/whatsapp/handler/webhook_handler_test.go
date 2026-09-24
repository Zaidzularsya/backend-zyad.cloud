package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

const testHMACKey = "hook-secret"

type fakeDirectory map[string]domain.DirectoryEntry

func (d fakeDirectory) Resolve(_ context.Context, name string) (domain.DirectoryEntry, error) {
	entry, ok := d[name]
	if !ok {
		return domain.DirectoryEntry{}, pgx.ErrNoRows
	}
	return entry, nil
}

type fakeEvents struct {
	stored map[string]repository.InsertWebhookEventParams
}

func (e *fakeEvents) Insert(_ context.Context, params repository.InsertWebhookEventParams) (bool, error) {
	if _, exists := e.stored[params.EventID]; exists {
		return false, nil
	}
	e.stored[params.EventID] = params
	return true, nil
}

func sign(body []byte, key string) string {
	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func newWebhookRouter(key string) (*gin.Engine, *fakeEvents) {
	gin.SetMode(gin.TestMode)
	events := &fakeEvents{stored: map[string]repository.InsertWebhookEventParams{}}
	directory := fakeDirectory{"zc_known_abc123": {SessionName: "zc_known_abc123", OrganizationID: "org-1", SessionID: "s-1"}}
	router := gin.New()
	NewWebhookHandler(directory, events, key, nil).RegisterRoutes(router.Group("/api/v1"))
	return router, events
}

func post(router *gin.Engine, body []byte, signature string) int {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/waha", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if signature != "" {
		request.Header.Set(platformwhatsapp.HeaderWebhookHMAC, signature)
		request.Header.Set(platformwhatsapp.HeaderWebhookHMACAlgorithm, "sha512")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code
}

var knownEvent = []byte(`{"id":"evt_01","event":"message.any","session":"zc_known_abc123","payload":{"id":"m1","body":"halo"}}`)

func TestWebhookStoresSignedEventOnce(t *testing.T) {
	router, events := newWebhookRouter(testHMACKey)

	if code := post(router, knownEvent, sign(knownEvent, testHMACKey)); code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	stored, ok := events.stored["evt_01"]
	if !ok || stored.SessionName != "zc_known_abc123" || stored.EventType != "message.any" || !bytes.Equal(stored.Payload, knownEvent) {
		t.Fatalf("stored = %+v", stored)
	}
	// WAHA retries: duplicates are acknowledged without a second row.
	if code := post(router, knownEvent, sign(knownEvent, testHMACKey)); code != http.StatusOK || len(events.stored) != 1 {
		t.Fatalf("duplicate: status = %d, stored = %d", code, len(events.stored))
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	router, events := newWebhookRouter(testHMACKey)
	for _, signature := range []string{"", "deadbeef", sign(knownEvent, "other-key")} {
		if code := post(router, knownEvent, signature); code != http.StatusUnauthorized {
			t.Fatalf("signature %q: status = %d, want 401", signature, code)
		}
	}
	if len(events.stored) != 0 {
		t.Fatal("event stored despite invalid signature")
	}
}

func TestWebhookNotConfigured(t *testing.T) {
	router, _ := newWebhookRouter("")
	if code := post(router, knownEvent, sign(knownEvent, "")); code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
}

func TestWebhookInvalidBody(t *testing.T) {
	router, _ := newWebhookRouter(testHMACKey)
	for _, body := range [][]byte{[]byte(`not json`), []byte(`{"event":"message.any","session":"zc_known_abc123"}`)} {
		if code := post(router, body, sign(body, testHMACKey)); code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, code)
		}
	}
}

func TestWebhookIgnoresUnknownSession(t *testing.T) {
	router, events := newWebhookRouter(testHMACKey)
	body := []byte(`{"id":"evt_02","event":"message","session":"session_zyad","payload":{}}`)
	if code := post(router, body, sign(body, testHMACKey)); code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(events.stored) != 0 {
		t.Fatal("event of an unknown session was stored")
	}
}
