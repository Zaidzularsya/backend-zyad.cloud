package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProviderWAHA = "waha"

	defaultWAHATimeout  = 15 * time.Second
	maxResponseBytes    = 1 << 20
	errorBodyPreviewLen = 512
)

// Session statuses reported by WAHA (SessionInfo.status / session.status event).
const (
	SessionStatusStopped                     = "STOPPED"
	SessionStatusStarting                    = "STARTING"
	SessionStatusScanQRCode                  = "SCAN_QR_CODE"
	SessionStatusPasskeyRequired             = "PASSKEY_REQUIRED"
	SessionStatusPasskeyConfirmationRequired = "PASSKEY_CONFIRMATION_REQUIRED"
	SessionStatusWorking                     = "WORKING"
	SessionStatusFailed                      = "FAILED"
)

type WAHAConfig struct {
	BaseURL string
	APIKey  string
	// DefaultSession is used by SendText when Message.Session is empty
	// (platform notifications).
	DefaultSession string
	Timeout        time.Duration
}

func (c WAHAConfig) HasCredentials() bool {
	return strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.APIKey) != ""
}

type WebhookConfig struct {
	URL     string
	Events  []string
	HMACKey string
}

type CreateSessionRequest struct {
	Name     string
	Start    bool
	Metadata map[string]string
	Webhooks []WebhookConfig
}

type SessionInfo struct {
	Name   string
	Status string
	Me     *Me
}

type Me struct {
	ID       string
	PushName string
}

// QRCode is the base64 PNG WAHA returns for format=image with
// Accept: application/json.
type QRCode struct {
	MimeType string
	Data     string
}

func (q QRCode) DataURL() string {
	return "data:" + q.MimeType + ";base64," + q.Data
}

type WAHAClient struct {
	config     WAHAConfig
	httpClient *http.Client
	now        func() time.Time
}

func NewWAHAClient(config WAHAConfig) *WAHAClient {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultWAHATimeout
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")

	return &WAHAClient{
		config:     config,
		httpClient: &http.Client{Timeout: timeout},
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// NewClientFromConfig returns the WAHA client when provider is "waha" and
// credentials are present; otherwise the NoopClient, so environments without
// WAHA keep working.
func NewClientFromConfig(provider string, config WAHAConfig) Client {
	if strings.EqualFold(strings.TrimSpace(provider), ProviderWAHA) && config.HasCredentials() {
		return NewWAHAClient(config)
	}
	return NewNoopClient()
}

type wahaWebhook struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	HMAC   *struct {
		Key string `json:"key"`
	} `json:"hmac,omitempty"`
}

type wahaSessionConfig struct {
	Metadata map[string]string `json:"metadata,omitempty"`
	Webhooks []wahaWebhook     `json:"webhooks,omitempty"`
}

type wahaSession struct {
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Me     *wahaMe `json:"me"`
}

type wahaMe struct {
	ID       string `json:"id"`
	PushName string `json:"pushName"`
}

func (s wahaSession) toSessionInfo() SessionInfo {
	info := SessionInfo{Name: s.Name, Status: s.Status}
	if s.Me != nil && s.Me.ID != "" {
		info.Me = &Me{ID: s.Me.ID, PushName: s.Me.PushName}
	}
	return info
}

func (c *WAHAClient) CreateSession(ctx context.Context, request CreateSessionRequest) (SessionInfo, error) {
	if err := validateSessionName(request.Name); err != nil {
		return SessionInfo{}, err
	}

	cfg := wahaSessionConfig{Metadata: request.Metadata}
	for _, webhook := range request.Webhooks {
		item := wahaWebhook{URL: webhook.URL, Events: webhook.Events}
		if webhook.HMACKey != "" {
			item.HMAC = &struct {
				Key string `json:"key"`
			}{Key: webhook.HMACKey}
		}
		cfg.Webhooks = append(cfg.Webhooks, item)
	}
	body := map[string]any{
		"name":   request.Name,
		"start":  request.Start,
		"config": cfg,
	}

	var response wahaSession
	if err := c.do(ctx, "create session", http.MethodPost, "/api/sessions", body, &response); err != nil {
		return SessionInfo{}, err
	}
	return response.toSessionInfo(), nil
}

func (c *WAHAClient) GetSession(ctx context.Context, name string) (SessionInfo, error) {
	if err := validateSessionName(name); err != nil {
		return SessionInfo{}, err
	}
	var response wahaSession
	if err := c.do(ctx, "get session", http.MethodGet, "/api/sessions/"+url.PathEscape(name), nil, &response); err != nil {
		return SessionInfo{}, err
	}
	return response.toSessionInfo(), nil
}

func (c *WAHAClient) StartSession(ctx context.Context, name string) (SessionInfo, error) {
	return c.sessionAction(ctx, name, "start")
}

func (c *WAHAClient) StopSession(ctx context.Context, name string) (SessionInfo, error) {
	return c.sessionAction(ctx, name, "stop")
}

// LogoutSession unlinks the WhatsApp device but keeps the WAHA session.
func (c *WAHAClient) LogoutSession(ctx context.Context, name string) (SessionInfo, error) {
	return c.sessionAction(ctx, name, "logout")
}

func (c *WAHAClient) sessionAction(ctx context.Context, name, action string) (SessionInfo, error) {
	if err := validateSessionName(name); err != nil {
		return SessionInfo{}, err
	}
	var response wahaSession
	path := "/api/sessions/" + url.PathEscape(name) + "/" + action
	if err := c.do(ctx, action+" session", http.MethodPost, path, nil, &response); err != nil {
		return SessionInfo{}, err
	}
	return response.toSessionInfo(), nil
}

// DeleteSession removes the session from WAHA. A session that is already gone
// is treated as deleted so callers can retry safely.
func (c *WAHAClient) DeleteSession(ctx context.Context, name string) error {
	if err := validateSessionName(name); err != nil {
		return err
	}
	err := c.do(ctx, "delete session", http.MethodDelete, "/api/sessions/"+url.PathEscape(name), nil, nil)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	return err
}

// GetMe returns the linked account, or nil when the session is not
// authenticated yet (WAHA answers 200 with an empty body in that case).
func (c *WAHAClient) GetMe(ctx context.Context, name string) (*Me, error) {
	if err := validateSessionName(name); err != nil {
		return nil, err
	}
	var response *wahaMe
	if err := c.do(ctx, "get me", http.MethodGet, "/api/sessions/"+url.PathEscape(name)+"/me", nil, &response); err != nil {
		return nil, err
	}
	if response == nil || response.ID == "" {
		return nil, nil
	}
	return &Me{ID: response.ID, PushName: response.PushName}, nil
}

func (c *WAHAClient) GetQR(ctx context.Context, name string) (QRCode, error) {
	if err := validateSessionName(name); err != nil {
		return QRCode{}, err
	}
	var response struct {
		MimeType string `json:"mimetype"`
		Data     string `json:"data"`
	}
	path := "/api/" + url.PathEscape(name) + "/auth/qr?format=image"
	if err := c.do(ctx, "get qr", http.MethodGet, path, nil, &response); err != nil {
		return QRCode{}, err
	}
	if response.Data == "" {
		return QRCode{}, fmt.Errorf("%w: empty qr response", ErrUpstream)
	}
	if response.MimeType == "" {
		response.MimeType = "image/png"
	}
	return QRCode{MimeType: response.MimeType, Data: response.Data}, nil
}

// RequestPairingCode asks WAHA for a phone pairing code (format "ABCD-ABCD").
// phone must be in international format; non-digits are stripped.
func (c *WAHAClient) RequestPairingCode(ctx context.Context, name, phone string) (string, error) {
	if err := validateSessionName(name); err != nil {
		return "", err
	}
	digits := digitsOnly(phone)
	if digits == "" {
		return "", fmt.Errorf("whatsapp: pairing phone number is required")
	}
	var response struct {
		Code string `json:"code"`
	}
	path := "/api/" + url.PathEscape(name) + "/auth/request-code"
	if err := c.do(ctx, "request pairing code", http.MethodPost, path, map[string]string{"phoneNumber": digits}, &response); err != nil {
		return "", err
	}
	if response.Code == "" {
		return "", fmt.Errorf("%w: empty pairing code", ErrUpstream)
	}
	return response.Code, nil
}

// SendText implements Client. Message.To may be a phone number (digits are
// extracted and sent to <digits>@c.us) or a full WAHA chat id.
func (c *WAHAClient) SendText(ctx context.Context, message Message) (Result, error) {
	session := strings.TrimSpace(message.Session)
	if session == "" {
		session = c.config.DefaultSession
	}
	if err := validateSessionName(session); err != nil {
		return Result{}, err
	}
	chatID := ToChatID(message.To)
	if chatID == "" {
		return Result{}, fmt.Errorf("whatsapp: recipient is required")
	}
	if strings.TrimSpace(message.Text) == "" {
		return Result{}, fmt.Errorf("whatsapp: message text is required")
	}

	body := map[string]any{
		"session": session,
		"chatId":  chatID,
		"text":    message.Text,
	}
	var response struct {
		ID        string  `json:"id"`
		Timestamp float64 `json:"timestamp"`
	}
	if err := c.do(ctx, "send text", http.MethodPost, "/api/sendText", body, &response); err != nil {
		return Result{}, err
	}

	sentAt := c.now()
	if response.Timestamp > 0 {
		sentAt = time.Unix(int64(response.Timestamp), 0).UTC()
	}
	return Result{
		Provider:  ProviderWAHA,
		MessageID: response.ID,
		Raw:       map[string]any{"session": session, "chat_id": chatID},
		SentAt:    sentAt,
	}, nil
}

// SendSeen marks messages in a chat as read. WAHA recommends it before
// replying to an inbound message to reduce the risk of the number being
// flagged.
func (c *WAHAClient) SendSeen(ctx context.Context, session, chatID string, messageIDs []string) error {
	if err := validateSessionName(session); err != nil {
		return err
	}
	body := map[string]any{"session": session, "chatId": chatID}
	if len(messageIDs) > 0 {
		body["messageIds"] = messageIDs
	}
	return c.do(ctx, "send seen", http.MethodPost, "/api/sendSeen", body, nil)
}

// ResolveLID maps a Linked ID (xxx@lid) to its phone chat id (xxx@c.us).
// It returns "" when WhatsApp does not expose the phone number.
func (c *WAHAClient) ResolveLID(ctx context.Context, session, lid string) (string, error) {
	if err := validateSessionName(session); err != nil {
		return "", err
	}
	var response struct {
		LID string  `json:"lid"`
		PN  *string `json:"pn"`
	}
	path := "/api/" + url.PathEscape(session) + "/lids/" + url.PathEscape(lid)
	if err := c.do(ctx, "resolve lid", http.MethodGet, path, nil, &response); err != nil {
		return "", err
	}
	if response.PN == nil {
		return "", nil
	}
	return *response.PN, nil
}

func (c *WAHAClient) do(ctx context.Context, operation, method, path string, body any, out any) error {
	if !c.config.HasCredentials() {
		return ErrNotConfigured
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode waha %s request: %w", operation, err)
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build waha %s request: %w", operation, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Api-Key", c.config.APIKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if isTimeout(err) {
			return fmt.Errorf("waha %s: %w", operation, ErrTimeout)
		}
		return fmt.Errorf("call waha %s: %w", operation, errors.Join(ErrUpstream, err))
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read waha %s response: %w", operation, err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &APIError{Operation: operation, StatusCode: response.StatusCode, Body: preview(responseBody)}
	}

	if out == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode waha %s response: %w", operation, err)
	}
	return nil
}

// ToChatID converts a phone number or chat id into a WAHA chat id. Existing
// chat ids are kept (with @s.whatsapp.net normalised to @c.us as WAHA
// requires); otherwise the digits become <digits>@c.us.
func ToChatID(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "@") {
		return strings.Replace(value, "@s.whatsapp.net", "@c.us", 1)
	}
	digits := digitsOnly(value)
	if digits == "" {
		return ""
	}
	return digits + "@c.us"
}

// validateSessionName restricts session names to [A-Za-z0-9_-]. The app
// generates [a-z0-9_] names; the wider set only admits operator-configured
// names such as WHATSAPP_API_SESSION. WAHA does not document its own rules and
// the name is used as a URL path segment.
func validateSessionName(name string) error {
	if name == "" {
		return fmt.Errorf("whatsapp: session name is required")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r >= 'A' && r <= 'Z') {
			return fmt.Errorf("whatsapp: invalid session name %q", name)
		}
	}
	return nil
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

func preview(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > errorBodyPreviewLen {
		return text[:errorBodyPreviewLen] + "..."
	}
	return text
}
