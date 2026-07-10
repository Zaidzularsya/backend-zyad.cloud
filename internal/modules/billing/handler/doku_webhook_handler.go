package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	billing "zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/platform/doku"
)

// DokuWebhookPaymentService is the slice of billing's PaymentService the DOKU
// notification handler needs: audit-trail event recording plus the idempotent
// invoice settlement path (which also activates subscription upgrades).
type DokuWebhookPaymentService interface {
	RecordProviderEvent(
		ctx context.Context,
		request dto.RecordPaymentProviderEventRequest,
	) (dto.PaymentEventResponse, error)
	MarkInvoicePaidByID(
		ctx context.Context,
		invoiceID string,
		request dto.MarkInvoicePaidRequest,
	) (dto.PaymentResponse, error)
}

type DokuWebhookHandler struct {
	payments  DokuWebhookPaymentService
	clientID  string
	secretKey string
	logger    *slog.Logger
}

func NewDokuWebhookHandler(
	payments DokuWebhookPaymentService,
	clientID string,
	secretKey string,
	logger *slog.Logger,
) *DokuWebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DokuWebhookHandler{
		payments:  payments,
		clientID:  strings.TrimSpace(clientID),
		secretKey: strings.TrimSpace(secretKey),
		logger:    logger,
	}
}

func (h *DokuWebhookHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/webhooks/doku", h.HandleNotification)
}

// dokuSuccessStatuses holds transaction.status values DOKU uses for settled
// payments. Verified against sandbox notifications; extend if DOKU introduces
// additional literals.
var dokuSuccessStatuses = map[string]struct{}{
	"SUCCESS": {},
	"PAID":    {},
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
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_BODY_INVALID",
			"failed to read notification body",
			http.StatusBadRequest,
		))
		return
	}

	clientID := strings.TrimSpace(c.GetHeader("Client-Id"))
	requestID := strings.TrimSpace(c.GetHeader("Request-Id"))
	requestTimestamp := strings.TrimSpace(c.GetHeader("Request-Timestamp"))
	signature := c.GetHeader("Signature")
	if clientID != h.clientID || !doku.VerifySignature(
		clientID,
		requestID,
		requestTimestamp,
		c.Request.URL.Path,
		body,
		signature,
		h.secretKey,
	) {
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_SIGNATURE_INVALID",
			"doku notification signature is invalid",
			http.StatusUnauthorized,
		))
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_BODY_INVALID",
			"notification body is not valid JSON",
			http.StatusBadRequest,
		))
		return
	}

	notification := parseDokuNotification(payload)
	if notification.InvoiceNumber == "" {
		corehttp.Fail(c, coreerrors.New(
			"PAYMENT_WEBHOOK_INVOICE_REQUIRED",
			"notification is missing order.invoice_number",
			http.StatusBadRequest,
		))
		return
	}

	status := strings.ToUpper(notification.Status)
	eventType := "doku_notification_" + strings.ToLower(defaultString(status, "unknown"))
	if _, err := h.payments.RecordProviderEvent(c.Request.Context(), dto.RecordPaymentProviderEventRequest{
		InvoiceID:       notification.InvoiceNumber,
		Provider:        string(model.PaymentProviderDoku),
		EventType:       eventType,
		ProviderEventID: "webhook:" + notification.InvoiceNumber + ":" + strings.ToLower(defaultString(status, "unknown")),
		Payload:         payload,
	}); err != nil && !isPaymentEventAlreadyProcessed(err) {
		h.logger.Error("record doku notification event failed", "error", err, "invoice", notification.InvoiceNumber)
		corehttp.Fail(c, err)
		return
	}

	if _, isSuccess := dokuSuccessStatuses[status]; !isSuccess {
		corehttp.OK(c, "doku notification recorded", gin.H{
			"invoice_number": notification.InvoiceNumber,
			"status":         status,
			"processed":      false,
		})
		return
	}

	if _, err := h.payments.MarkInvoicePaidByID(c.Request.Context(), notification.InvoiceNumber, dto.MarkInvoicePaidRequest{
		Provider:          string(model.PaymentProviderDoku),
		ProviderReference: notification.ProviderReference,
		PaymentMethod:     notification.PaymentMethod,
		Amount:            notification.Amount,
		Currency:          notification.Currency,
		PaidAt:            notification.PaidAt,
		RawPayload:        payload,
	}); err != nil {
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) {
			switch appErr.Code {
			case billing.ErrCodePaymentAlreadyProcessed:
				// Retried notification for an already settled invoice.
				corehttp.OK(c, "doku notification already processed", gin.H{
					"invoice_number": notification.InvoiceNumber,
					"processed":      true,
				})
				return
			case billing.ErrCodeInvoiceNotFound:
				// Unknown invoice: acknowledge so DOKU stops retrying a
				// notification we can never process.
				h.logger.Warn("doku notification for unknown invoice", "invoice", notification.InvoiceNumber)
				corehttp.OK(c, "doku notification ignored (unknown invoice)", gin.H{
					"invoice_number": notification.InvoiceNumber,
					"processed":      false,
				})
				return
			}
		}
		h.logger.Error("process doku notification failed", "error", err, "invoice", notification.InvoiceNumber)
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "doku notification processed", gin.H{
		"invoice_number": notification.InvoiceNumber,
		"processed":      true,
	})
}

type dokuNotification struct {
	InvoiceNumber     string
	Status            string
	Amount            string
	Currency          string
	PaymentMethod     string
	ProviderReference string
	PaidAt            *string
}

// parseDokuNotification extracts the fields we act on from a DOKU Checkout
// HTTP notification. Field locations follow DOKU's notification payload
// (order.invoice_number, order.amount, transaction.status, channel /
// service.id, transaction.original_request_id, transaction.date); lookups are
// defensive because DOKU varies the envelope per payment channel.
func parseDokuNotification(payload map[string]any) dokuNotification {
	notification := dokuNotification{}
	order := mapValue(payload, "order")
	transaction := mapValue(payload, "transaction")

	notification.InvoiceNumber = firstStringValue(
		stringValue(order, "invoice_number"),
		stringValue(payload, "invoice_number"),
	)
	notification.Status = firstStringValue(
		stringValue(transaction, "status"),
		stringValue(payload, "transaction_status"),
		stringValue(payload, "status"),
	)
	notification.Amount = firstStringValue(
		numberOrStringValue(order, "amount"),
		numberOrStringValue(transaction, "amount"),
	)
	notification.Currency = firstStringValue(
		stringValue(order, "currency"),
		stringValue(transaction, "currency"),
	)
	notification.PaymentMethod = firstStringValue(
		stringValue(mapValue(payload, "channel"), "id"),
		stringValue(mapValue(payload, "service"), "id"),
		stringValue(mapValue(payload, "acquirer"), "id"),
	)
	notification.ProviderReference = firstStringValue(
		stringValue(transaction, "original_request_id"),
		stringValue(order, "session_id"),
		stringValue(payload, "token_id"),
	)
	if paidAtRaw := stringValue(transaction, "date"); paidAtRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, paidAtRaw); err == nil {
			formatted := parsed.UTC().Format(time.RFC3339)
			notification.PaidAt = &formatted
		}
	}
	return notification
}

func mapValue(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	value, ok := payload[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
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
	default:
		if value == nil {
			return ""
		}
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

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isPaymentEventAlreadyProcessed(err error) bool {
	var appErr *coreerrors.AppError
	return errors.As(err, &appErr) && appErr.Code == billing.ErrCodePaymentEventAlreadyProcessed
}
