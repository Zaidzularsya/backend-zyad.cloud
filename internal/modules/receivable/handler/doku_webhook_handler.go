package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/doku"
)

// receivableInvoicePrefix harus sama dengan service.DokuNumberPrefix.
const receivableInvoicePrefix = "RCV-"

// DokuProcessor memproses notifikasi bernomor RCV- (invoice receivable).
type DokuProcessor interface {
	HandleNotification(ctx context.Context, n service.DokuNotification) (bool, error)
}

// DokuWebhookHandler menerima notifikasi DOKU. URL tetap POST /webhooks/doku sejak
// modul billing dipensiunkan (R4-S5) agar konfigurasi di dashboard DOKU tidak berubah.
type DokuWebhookHandler struct {
	processor DokuProcessor
	clientID  string
	secretKey string
	logger    *slog.Logger
}

func NewDokuWebhookHandler(processor DokuProcessor, clientID, secretKey string, logger *slog.Logger) *DokuWebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DokuWebhookHandler{
		processor: processor,
		clientID:  strings.TrimSpace(clientID),
		secretKey: strings.TrimSpace(secretKey),
		logger:    logger,
	}
}

func (h *DokuWebhookHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/webhooks/doku", h.HandleNotification)
}

func (h *DokuWebhookHandler) HandleNotification(c *gin.Context) {
	if h.secretKey == "" || h.clientID == "" {
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_NOT_CONFIGURED",
			"doku webhook credentials are not configured",
			http.StatusServiceUnavailable,
		))
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		corehttp.Fail(c, coreerrors.New("PAYMENT_WEBHOOK_BODY_INVALID", "failed to read notification body", http.StatusBadRequest))
		return
	}

	clientID := strings.TrimSpace(c.GetHeader("Client-Id"))
	requestID := strings.TrimSpace(c.GetHeader("Request-Id"))
	requestTimestamp := strings.TrimSpace(c.GetHeader("Request-Timestamp"))
	signature := c.GetHeader("Signature")
	if clientID != h.clientID || !doku.VerifySignature(
		clientID, requestID, requestTimestamp, c.Request.URL.Path, body, signature, h.secretKey,
	) {
		// Dump header/payload agar skema yang tidak cocok (mis. SNAP dengan X-SIGNATURE)
		// bisa didiagnosis dari log saja.
		h.logger.Warn("doku notification rejected: signature mismatch",
			"client_id_header", clientID,
			"request_id", requestID,
			"request_timestamp", requestTimestamp,
			"has_signature", signature != "",
			"x_signature", c.GetHeader("X-SIGNATURE") != "",
			"x_timestamp", c.GetHeader("X-TIMESTAMP"),
			"x_partner_id", c.GetHeader("X-PARTNER-ID"),
			"path", c.Request.URL.Path,
			"body_preview", string(body[:min(len(body), 800)]),
		)
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_SIGNATURE_INVALID",
			"doku notification signature is invalid",
			http.StatusUnauthorized,
		))
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		corehttp.Fail(c, coreerrors.New("PAYMENT_WEBHOOK_BODY_INVALID", "notification body is not valid JSON", http.StatusBadRequest))
		return
	}

	n := parseDokuNotification(payload)
	if n.InvoiceNumber == "" {
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_INVOICE_REQUIRED",
			"notification is missing order.invoice_number",
			http.StatusBadRequest,
		))
		return
	}

	if h.processor == nil || !strings.HasPrefix(n.InvoiceNumber, receivableInvoicePrefix) {
		// Retry DOKU untuk invoice billing platform lama (sudah dipensiunkan): ack agar DOKU berhenti.
		h.logger.Warn("doku notification for retired billing invoice", "invoice_number", n.InvoiceNumber)
		corehttp.OK(c, "doku notification ignored", gin.H{
			"invoice_number": n.InvoiceNumber,
			"status":         "ignored",
		})
		return
	}

	event := service.DokuNotification{
		InvoiceNumber: n.InvoiceNumber, Status: n.Status, Amount: n.Amount,
		ProviderReference: n.ProviderReference, PaymentMethod: n.PaymentMethod, Payload: payload,
	}
	if n.PaidAt != nil {
		if parsed, err := time.Parse(time.RFC3339, *n.PaidAt); err == nil {
			event.PaidAt = &parsed
		}
	}
	// Galat tak terduga → 500 agar DOKU mengirim ulang; selain itu 200.
	processed, err := h.processor.HandleNotification(c.Request.Context(), event)
	if err != nil {
		h.logger.Error("process doku receivable notification failed", "error", err, "invoice", n.InvoiceNumber)
		corehttp.Fail(c, err)
		return
	}
	message := "doku notification recorded"
	if processed {
		message = "doku notification processed"
	}
	corehttp.OK(c, message, gin.H{
		"invoice_number": n.InvoiceNumber,
		"status":         strings.ToUpper(n.Status),
		"processed":      processed,
	})
}

type dokuNotification struct {
	InvoiceNumber     string
	Status            string
	Amount            string
	PaymentMethod     string
	ProviderReference string
	PaidAt            *string
}

// parseDokuNotification mengambil field yang dipakai dari notifikasi HTTP DOKU Checkout.
// Lookup bersifat defensif karena DOKU memvariasikan envelope per kanal pembayaran.
func parseDokuNotification(payload map[string]any) dokuNotification {
	n := dokuNotification{}
	order := mapValue(payload, "order")
	transaction := mapValue(payload, "transaction")

	n.InvoiceNumber = firstStringValue(stringValue(order, "invoice_number"), stringValue(payload, "invoice_number"))
	n.Status = firstStringValue(
		stringValue(transaction, "status"),
		stringValue(payload, "transaction_status"),
		stringValue(payload, "status"),
	)
	n.Amount = firstStringValue(numberOrStringValue(order, "amount"), numberOrStringValue(transaction, "amount"))
	n.PaymentMethod = firstStringValue(
		stringValue(mapValue(payload, "channel"), "id"),
		stringValue(mapValue(payload, "service"), "id"),
		stringValue(mapValue(payload, "acquirer"), "id"),
	)
	n.ProviderReference = firstStringValue(
		stringValue(transaction, "original_request_id"),
		stringValue(order, "session_id"),
		stringValue(payload, "token_id"),
	)
	if raw := stringValue(transaction, "date"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			formatted := parsed.UTC().Format(time.RFC3339)
			n.PaidAt = &formatted
		}
	}
	return n
}

func mapValue(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	value, _ := payload[key].(map[string]any)
	return value
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func numberOrStringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	switch value := payload[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		return value.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", value)
	}
}

func firstStringValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
