package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAPIKey = "test-api-key"

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	APIKey string
	Accept string
	Body   map[string]any
}

// newTestServer answers every request with status and response, recording the
// last request for assertions.
func newTestServer(t *testing.T, status int, response string) (*WAHAClient, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Query = r.URL.RawQuery
		captured.APIKey = r.Header.Get("X-Api-Key")
		captured.Accept = r.Header.Get("Accept")
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &captured.Body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	client := NewWAHAClient(WAHAConfig{BaseURL: server.URL + "/", APIKey: testAPIKey, DefaultSession: "default"})
	return client, captured
}

func TestWAHACreateSession(t *testing.T) {
	client, captured := newTestServer(t, http.StatusCreated, `{"name":"zc_abc_123456","status":"STARTING"}`)

	info, err := client.CreateSession(context.Background(), CreateSessionRequest{
		Name:     "zc_abc_123456",
		Start:    true,
		Metadata: map[string]string{"zyad.organization_id": "org-1"},
		Webhooks: []WebhookConfig{{
			URL:     "https://api.example.test/api/v1/webhooks/waha",
			Events:  []string{"session.status", "message"},
			HMACKey: "hook-secret",
		}},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if info.Name != "zc_abc_123456" || info.Status != SessionStatusStarting {
		t.Fatalf("info = %+v", info)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/sessions" {
		t.Fatalf("request = %s %s", captured.Method, captured.Path)
	}
	if captured.APIKey != testAPIKey {
		t.Fatalf("X-Api-Key = %q", captured.APIKey)
	}
	config := captured.Body["config"].(map[string]any)
	webhook := config["webhooks"].([]any)[0].(map[string]any)
	if webhook["hmac"].(map[string]any)["key"] != "hook-secret" {
		t.Fatalf("webhook hmac = %#v", webhook["hmac"])
	}
	if config["metadata"].(map[string]any)["zyad.organization_id"] != "org-1" {
		t.Fatalf("metadata = %#v", config["metadata"])
	}
	if captured.Body["start"] != true {
		t.Fatalf("start = %#v", captured.Body["start"])
	}
}

func TestWAHAGetSessionWithMe(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK,
		`{"name":"zc_a_1","status":"WORKING","me":{"id":"628123@c.us","pushName":"Sales"}}`)

	info, err := client.GetSession(context.Background(), "zc_a_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if captured.Method != http.MethodGet || captured.Path != "/api/sessions/zc_a_1" {
		t.Fatalf("request = %s %s", captured.Method, captured.Path)
	}
	if info.Status != SessionStatusWorking || info.Me == nil || info.Me.ID != "628123@c.us" || info.Me.PushName != "Sales" {
		t.Fatalf("info = %+v me=%+v", info, info.Me)
	}
}

func TestWAHASessionActions(t *testing.T) {
	tests := []struct {
		name   string
		action func(*WAHAClient) (SessionInfo, error)
		path   string
	}{
		{"start", func(c *WAHAClient) (SessionInfo, error) { return c.StartSession(context.Background(), "zc_a_1") }, "/api/sessions/zc_a_1/start"},
		{"stop", func(c *WAHAClient) (SessionInfo, error) { return c.StopSession(context.Background(), "zc_a_1") }, "/api/sessions/zc_a_1/stop"},
		{"logout", func(c *WAHAClient) (SessionInfo, error) { return c.LogoutSession(context.Background(), "zc_a_1") }, "/api/sessions/zc_a_1/logout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, captured := newTestServer(t, http.StatusCreated, `{"name":"zc_a_1","status":"STOPPED"}`)
			info, err := tt.action(client)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if captured.Method != http.MethodPost || captured.Path != tt.path {
				t.Fatalf("request = %s %s, want POST %s", captured.Method, captured.Path, tt.path)
			}
			if info.Status != SessionStatusStopped {
				t.Fatalf("status = %q", info.Status)
			}
		})
	}
}

func TestWAHADeleteSession(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK, ``)
	if err := client.DeleteSession(context.Background(), "zc_a_1"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if captured.Method != http.MethodDelete || captured.Path != "/api/sessions/zc_a_1" {
		t.Fatalf("request = %s %s", captured.Method, captured.Path)
	}
}

func TestWAHADeleteSessionAlreadyGoneIsNotAnError(t *testing.T) {
	client, _ := newTestServer(t, http.StatusNotFound, `{"message":"Session not found"}`)
	if err := client.DeleteSession(context.Background(), "zc_a_1"); err != nil {
		t.Fatalf("DeleteSession() error = %v, want nil for 404", err)
	}
}

func TestWAHAGetMe(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK, `{"id":"628123@c.us","pushName":"Sales"}`)
	me, err := client.GetMe(context.Background(), "zc_a_1")
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if captured.Path != "/api/sessions/zc_a_1/me" {
		t.Fatalf("path = %s", captured.Path)
	}
	if me == nil || me.ID != "628123@c.us" {
		t.Fatalf("me = %+v", me)
	}
}

func TestWAHAGetMeNotAuthenticated(t *testing.T) {
	client, _ := newTestServer(t, http.StatusOK, `null`)
	me, err := client.GetMe(context.Background(), "zc_a_1")
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if me != nil {
		t.Fatalf("me = %+v, want nil", me)
	}
}

func TestWAHAGetQR(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK, `{"mimetype":"image/png","data":"iVBORw0KGgo="}`)
	qr, err := client.GetQR(context.Background(), "zc_a_1")
	if err != nil {
		t.Fatalf("GetQR() error = %v", err)
	}
	if captured.Path != "/api/zc_a_1/auth/qr" || captured.Query != "format=image" {
		t.Fatalf("request = %s?%s", captured.Path, captured.Query)
	}
	if captured.Accept != "application/json" {
		t.Fatalf("Accept = %q, want application/json (base64 response)", captured.Accept)
	}
	if qr.DataURL() != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("DataURL() = %q", qr.DataURL())
	}
}

func TestWAHARequestPairingCode(t *testing.T) {
	client, captured := newTestServer(t, http.StatusCreated, `{"code":"ABCD-EFGH"}`)
	code, err := client.RequestPairingCode(context.Background(), "zc_a_1", "+62 812-3456-789")
	if err != nil {
		t.Fatalf("RequestPairingCode() error = %v", err)
	}
	if code != "ABCD-EFGH" {
		t.Fatalf("code = %q", code)
	}
	if captured.Path != "/api/zc_a_1/auth/request-code" || captured.Body["phoneNumber"] != "628123456789" {
		t.Fatalf("request = %s body=%#v", captured.Path, captured.Body)
	}
}

func TestWAHASendText(t *testing.T) {
	client, captured := newTestServer(t, http.StatusCreated,
		`{"id":"true_628123456789@c.us_AAAA","timestamp":1790000000,"body":"Halo"}`)

	result, err := client.SendText(context.Background(), Message{Session: "zc_a_1", To: "+628123456789", Text: "Halo"})
	if err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if captured.Path != "/api/sendText" {
		t.Fatalf("path = %s", captured.Path)
	}
	if captured.Body["session"] != "zc_a_1" || captured.Body["chatId"] != "628123456789@c.us" || captured.Body["text"] != "Halo" {
		t.Fatalf("body = %#v", captured.Body)
	}
	if result.Provider != ProviderWAHA || result.MessageID != "true_628123456789@c.us_AAAA" {
		t.Fatalf("result = %+v", result)
	}
	if !result.SentAt.Equal(time.Unix(1790000000, 0).UTC()) {
		t.Fatalf("SentAt = %v", result.SentAt)
	}
}

func TestWAHASendTextUsesDefaultSession(t *testing.T) {
	client, captured := newTestServer(t, http.StatusCreated, `{"id":"x"}`)
	if _, err := client.SendText(context.Background(), Message{To: "628123@c.us", Text: "Hi"}); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if captured.Body["session"] != "default" || captured.Body["chatId"] != "628123@c.us" {
		t.Fatalf("body = %#v", captured.Body)
	}
}

func TestWAHASendSeen(t *testing.T) {
	client, captured := newTestServer(t, http.StatusCreated, `{}`)
	if err := client.SendSeen(context.Background(), "zc_a_1", "628123@c.us", []string{"false_628123@c.us_AAA"}); err != nil {
		t.Fatalf("SendSeen() error = %v", err)
	}
	if captured.Path != "/api/sendSeen" || captured.Body["chatId"] != "628123@c.us" {
		t.Fatalf("request = %s body=%#v", captured.Path, captured.Body)
	}
	if ids := captured.Body["messageIds"].([]any); len(ids) != 1 {
		t.Fatalf("messageIds = %#v", ids)
	}
}

func TestWAHAResolveLID(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK, `{"lid":"111@lid","pn":"628123@c.us"}`)
	pn, err := client.ResolveLID(context.Background(), "zc_a_1", "111@lid")
	if err != nil {
		t.Fatalf("ResolveLID() error = %v", err)
	}
	if captured.Path != "/api/zc_a_1/lids/111@lid" || pn != "628123@c.us" {
		t.Fatalf("path = %s pn = %q", captured.Path, pn)
	}
}

func TestWAHAResolveLIDWithoutPhone(t *testing.T) {
	client, _ := newTestServer(t, http.StatusOK, `{"lid":"111@lid","pn":null}`)
	pn, err := client.ResolveLID(context.Background(), "zc_a_1", "111@lid")
	if err != nil || pn != "" {
		t.Fatalf("ResolveLID() = %q, %v; want empty, nil", pn, err)
	}
}

func TestWAHAErrorMapping(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrSessionNotFound},
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrUnauthorized},
		{http.StatusInternalServerError, ErrUpstream},
		{http.StatusUnprocessableEntity, ErrUpstream},
	}
	for _, tt := range tests {
		client, _ := newTestServer(t, tt.status, `{"message":"boom"}`)
		_, err := client.GetSession(context.Background(), "zc_a_1")
		if !errors.Is(err, tt.want) {
			t.Fatalf("status %d: error = %v, want %v", tt.status, err, tt.want)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status || !strings.Contains(apiErr.Body, "boom") {
			t.Fatalf("status %d: APIError = %+v", tt.status, apiErr)
		}
		if strings.Contains(err.Error(), testAPIKey) {
			t.Fatalf("error leaks api key: %v", err)
		}
	}
}

func TestWAHATimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	t.Cleanup(server.Close)
	client := NewWAHAClient(WAHAConfig{BaseURL: server.URL, APIKey: testAPIKey, Timeout: 20 * time.Millisecond})

	_, err := client.GetSession(context.Background(), "zc_a_1")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
}

func TestWAHANotConfigured(t *testing.T) {
	client := NewWAHAClient(WAHAConfig{})
	if _, err := client.GetSession(context.Background(), "zc_a_1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestWAHARejectsUnsafeSessionName(t *testing.T) {
	client, captured := newTestServer(t, http.StatusOK, `{}`)
	for _, name := range []string{"", "../admin", "a/b", "a b", "a?x=1"} {
		if _, err := client.GetSession(context.Background(), name); err == nil {
			t.Fatalf("GetSession(%q) error = nil", name)
		}
	}
	if captured.Path != "" {
		t.Fatalf("request was sent for invalid name: %s", captured.Path)
	}
}

func TestNewClientFromConfig(t *testing.T) {
	configured := WAHAConfig{BaseURL: "https://waha.example.test", APIKey: "k"}

	if _, ok := NewClientFromConfig("waha", configured).(*WAHAClient); !ok {
		t.Fatal("provider waha with credentials should return *WAHAClient")
	}
	if _, ok := NewClientFromConfig("WAHA", configured).(*WAHAClient); !ok {
		t.Fatal("provider match should be case-insensitive")
	}
	if _, ok := NewClientFromConfig("waha", WAHAConfig{}).(NoopClient); !ok {
		t.Fatal("waha without credentials should fall back to NoopClient")
	}
	if _, ok := NewClientFromConfig("noop", configured).(NoopClient); !ok {
		t.Fatal("provider noop should return NoopClient")
	}
	if _, ok := NewClientFromConfig("", configured).(NoopClient); !ok {
		t.Fatal("empty provider should return NoopClient")
	}
}

func TestToChatID(t *testing.T) {
	tests := map[string]string{
		"+62 812-3456-789":      "628123456789@c.us",
		"628123@c.us":           "628123@c.us",
		"628123@s.whatsapp.net": "628123@c.us",
		"111@lid":               "111@lid",
		"":                      "",
		"abc":                   "",
	}
	for input, want := range tests {
		if got := ToChatID(input); got != want {
			t.Fatalf("ToChatID(%q) = %q, want %q", input, got, want)
		}
	}
}
