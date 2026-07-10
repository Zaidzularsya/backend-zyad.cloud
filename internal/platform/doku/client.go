package doku

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SandboxBaseURL    = "https://api-sandbox.doku.com"
	ProductionBaseURL = "https://api.doku.com"

	checkoutPaymentPath = "/checkout/v1/payment"

	// expiredDateLayout is DOKU's payment.expired_date format (UTC+7).
	expiredDateLayout = "20060102150405"

	signaturePrefix = "HMACSHA256="
)

// dokuTimezone is the fixed UTC+7 offset DOKU uses for expired_date values.
var dokuTimezone = time.FixedZone("WIB", 7*60*60)

type Config struct {
	BaseURL   string
	ClientID  string
	SecretKey string
	// FrontendURL is only used by the NoopClient to fabricate a clickable
	// payment URL when no credentials are configured.
	FrontendURL string
}

func (c Config) HasCredentials() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.SecretKey) != ""
}

// CreatePaymentRequest describes a DOKU Checkout payment page request.
// InvoiceNumber is our own merchant-generated identifier (max 64 chars);
// DOKU echoes it back verbatim in redirects and notifications.
type CreatePaymentRequest struct {
	InvoiceNumber         string
	Amount                int64
	Currency              string
	PaymentDueDateMinutes int
	CallbackURL           string
}

// Payment is the hosted checkout session DOKU created for the request.
type Payment struct {
	TokenID        string
	SessionID      string
	PaymentURL     string
	ExpiredDate    time.Time
	PaymentDueDate int
}

type Client interface {
	CreatePayment(ctx context.Context, request CreatePaymentRequest) (Payment, error)
}

// Digest returns the base64-encoded SHA256 of a raw request/notification body,
// as used in DOKU's Digest header and signature component.
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Signature computes the DOKU HMAC-SHA256 signature header value
// ("HMACSHA256=<base64>") over the canonical component string. digest may be
// empty for requests without a body (the Digest line is then omitted).
func Signature(clientID, requestID, requestTimestamp, requestTarget, digest, secretKey string) string {
	lines := []string{
		"Client-Id:" + clientID,
		"Request-Id:" + requestID,
		"Request-Timestamp:" + requestTimestamp,
		"Request-Target:" + requestTarget,
	}
	if digest != "" {
		lines = append(lines, "Digest:"+digest)
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(strings.Join(lines, "\n")))
	return signaturePrefix + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifySignature recomputes the signature for an incoming DOKU notification
// and compares it in constant time against the received Signature header.
func VerifySignature(
	clientID, requestID, requestTimestamp, requestTarget string,
	body []byte,
	receivedSignature, secretKey string,
) bool {
	expected := Signature(clientID, requestID, requestTimestamp, requestTarget, Digest(body), secretKey)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(receivedSignature)))
}

type HTTPClient struct {
	config     Config
	httpClient *http.Client
	now        func() time.Time
	newUUID    func() string
}

func NewHTTPClient(config Config) *HTTPClient {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = SandboxBaseURL
	}
	return &HTTPClient{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
		newUUID:    newRequestID,
	}
}

// newRequestID returns a unique random identifier for the Request-Id header
// (DOKU allows any unique string up to 128 characters).
func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

type checkoutRequestBody struct {
	Order   checkoutOrder   `json:"order"`
	Payment checkoutPayment `json:"payment"`
}

type checkoutOrder struct {
	Amount        int64  `json:"amount"`
	InvoiceNumber string `json:"invoice_number"`
	Currency      string `json:"currency,omitempty"`
	CallbackURL   string `json:"callback_url,omitempty"`
}

type checkoutPayment struct {
	PaymentDueDate int `json:"payment_due_date"`
}

type checkoutResponseBody struct {
	Message  []string `json:"message"`
	Response struct {
		Order struct {
			InvoiceNumber string `json:"invoice_number"`
			SessionID     string `json:"session_id"`
		} `json:"order"`
		Payment struct {
			TokenID        string `json:"token_id"`
			URL            string `json:"url"`
			ExpiredDate    string `json:"expired_date"`
			PaymentDueDate int    `json:"payment_due_date"`
		} `json:"payment"`
	} `json:"response"`
	ErrorMessages []string `json:"error_messages"`
}

func (c *HTTPClient) CreatePayment(ctx context.Context, request CreatePaymentRequest) (Payment, error) {
	if !c.config.HasCredentials() {
		return Payment{}, fmt.Errorf("doku client credentials are not configured")
	}
	invoiceNumber := strings.TrimSpace(request.InvoiceNumber)
	if invoiceNumber == "" || len(invoiceNumber) > 64 {
		return Payment{}, fmt.Errorf("doku invoice number must be 1-64 characters")
	}
	if request.Amount <= 0 {
		return Payment{}, fmt.Errorf("doku payment amount must be positive")
	}
	dueDate := request.PaymentDueDateMinutes
	if dueDate <= 0 {
		dueDate = 60
	}

	body, err := json.Marshal(checkoutRequestBody{
		Order: checkoutOrder{
			Amount:        request.Amount,
			InvoiceNumber: invoiceNumber,
			Currency:      strings.TrimSpace(request.Currency),
			CallbackURL:   strings.TrimSpace(request.CallbackURL),
		},
		Payment: checkoutPayment{PaymentDueDate: dueDate},
	})
	if err != nil {
		return Payment{}, fmt.Errorf("marshal doku checkout request: %w", err)
	}

	endpoint := strings.TrimRight(c.config.BaseURL, "/") + checkoutPaymentPath
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Payment{}, fmt.Errorf("build doku checkout request: %w", err)
	}

	requestID := c.newUUID()
	requestTimestamp := c.now().UTC().Format("2006-01-02T15:04:05Z")
	digest := Digest(body)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Client-Id", c.config.ClientID)
	httpRequest.Header.Set("Request-Id", requestID)
	httpRequest.Header.Set("Request-Timestamp", requestTimestamp)
	httpRequest.Header.Set("Digest", digest)
	httpRequest.Header.Set("Signature", Signature(
		c.config.ClientID,
		requestID,
		requestTimestamp,
		checkoutPaymentPath,
		digest,
		c.config.SecretKey,
	))

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return Payment{}, fmt.Errorf("call doku checkout: %w", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
	if err != nil {
		return Payment{}, fmt.Errorf("read doku checkout response: %w", err)
	}
	var parsed checkoutResponseBody
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Payment{}, fmt.Errorf(
			"decode doku checkout response (status %d): %w",
			httpResponse.StatusCode,
			err,
		)
	}
	if httpResponse.StatusCode != http.StatusOK {
		message := strings.Join(parsed.ErrorMessages, "; ")
		if message == "" {
			message = strings.Join(parsed.Message, "; ")
		}
		return Payment{}, fmt.Errorf("doku checkout failed (status %d): %s", httpResponse.StatusCode, message)
	}
	if strings.TrimSpace(parsed.Response.Payment.URL) == "" {
		return Payment{}, fmt.Errorf("doku checkout response is missing payment url")
	}

	payment := Payment{
		TokenID:        parsed.Response.Payment.TokenID,
		SessionID:      parsed.Response.Order.SessionID,
		PaymentURL:     parsed.Response.Payment.URL,
		PaymentDueDate: parsed.Response.Payment.PaymentDueDate,
	}
	if raw := strings.TrimSpace(parsed.Response.Payment.ExpiredDate); raw != "" {
		expired, parseErr := time.ParseInLocation(expiredDateLayout, raw, dokuTimezone)
		if parseErr == nil {
			payment.ExpiredDate = expired.UTC()
		}
	}
	if payment.ExpiredDate.IsZero() {
		payment.ExpiredDate = c.now().UTC().Add(time.Duration(dueDate) * time.Minute)
	}
	return payment, nil
}

// NoopClient fabricates a payment URL pointing straight at the frontend's
// checkout success page so the full order flow stays clickable in
// environments without DOKU credentials (CI, local dev).
type NoopClient struct {
	FrontendURL string
	now         func() time.Time
}

func NewNoopClient(frontendURL string) NoopClient {
	return NoopClient{FrontendURL: frontendURL, now: time.Now}
}

func (c NoopClient) CreatePayment(_ context.Context, request CreatePaymentRequest) (Payment, error) {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	dueDate := request.PaymentDueDateMinutes
	if dueDate <= 0 {
		dueDate = 60
	}
	base := strings.TrimRight(strings.TrimSpace(c.FrontendURL), "/")
	paymentURL := base + "/app/checkout/success?invoice=" +
		url.QueryEscape(strings.TrimSpace(request.InvoiceNumber)) + "&simulated=true"
	return Payment{
		TokenID:        "noop-" + strings.TrimSpace(request.InvoiceNumber),
		SessionID:      "noop",
		PaymentURL:     paymentURL,
		ExpiredDate:    now().UTC().Add(time.Duration(dueDate) * time.Minute),
		PaymentDueDate: dueDate,
	}, nil
}

// NewClientFromConfig returns the HTTP client when credentials are configured
// and the Noop fallback otherwise.
func NewClientFromConfig(config Config) Client {
	if config.HasCredentials() {
		return NewHTTPClient(config)
	}
	return NewNoopClient(config.FrontendURL)
}
