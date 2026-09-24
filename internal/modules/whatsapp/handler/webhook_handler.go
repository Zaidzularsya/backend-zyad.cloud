package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

const maxWebhookBody = 1 << 20

type WebhookSessionDirectory interface {
	Resolve(ctx context.Context, sessionName string) (domain.DirectoryEntry, error)
}

type WebhookEventInserter interface {
	Insert(ctx context.Context, params repository.InsertWebhookEventParams) (bool, error)
}

// WebhookHandler receives WAHA webhooks. It only authenticates and stores
// events; the worker processes them (InboundProcessor), so WAHA gets a fast
// 2xx and retries only on real failures.
type WebhookHandler struct {
	directory WebhookSessionDirectory
	events    WebhookEventInserter
	hmacKey   string
	log       *slog.Logger
}

func NewWebhookHandler(directory WebhookSessionDirectory, events WebhookEventInserter, hmacKey string, log *slog.Logger) *WebhookHandler {
	if log == nil {
		log = slog.Default()
	}
	return &WebhookHandler{directory: directory, events: events, hmacKey: strings.TrimSpace(hmacKey), log: log}
}

// RegisterRoutes mounts the public endpoint on the bare /api/v1 group (no
// auth or tenant middleware), like the DOKU webhook.
func (h *WebhookHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/webhooks/waha", h.Receive)
}

type webhookEnvelope struct {
	ID      string `json:"id"`
	Event   string `json:"event"`
	Session string `json:"session"`
}

func (h *WebhookHandler) Receive(c *gin.Context) {
	if h.hmacKey == "" {
		corehttp.Fail(c, coreerrors.New("WHATSAPP_WEBHOOK_NOT_CONFIGURED", "whatsapp webhook is not configured", http.StatusServiceUnavailable))
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		corehttp.Fail(c, coreerrors.New("WHATSAPP_WEBHOOK_BODY_INVALID", "failed to read webhook body", http.StatusBadRequest))
		return
	}

	if err := platformwhatsapp.VerifyWebhookSignature(body, c.Request.Header, h.hmacKey); err != nil {
		// No body in the log: it contains message content (personal data).
		h.log.Warn("waha webhook rejected: invalid signature",
			"request_id", c.GetHeader(platformwhatsapp.HeaderWebhookRequestID),
			"has_signature", c.GetHeader(platformwhatsapp.HeaderWebhookHMAC) != "",
		)
		corehttp.Fail(c, coreerrors.New("WHATSAPP_WEBHOOK_SIGNATURE_INVALID", "webhook signature is invalid", http.StatusUnauthorized))
		return
	}

	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil ||
		strings.TrimSpace(envelope.ID) == "" || strings.TrimSpace(envelope.Session) == "" || strings.TrimSpace(envelope.Event) == "" {
		corehttp.Fail(c, coreerrors.New("WHATSAPP_WEBHOOK_BODY_INVALID", "webhook body must be a WAHA event with id, event, and session", http.StatusBadRequest))
		return
	}

	if _, err := h.directory.Resolve(c.Request.Context(), envelope.Session); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not an app-managed session (or already deleted): acknowledge so
			// WAHA stops retrying, but do not store it.
			h.log.Info("waha webhook ignored: unknown session", "session", envelope.Session, "event", envelope.Event)
			corehttp.OK(c, "ignored", nil)
			return
		}
		corehttp.Fail(c, err)
		return
	}

	if _, err := h.events.Insert(c.Request.Context(), repository.InsertWebhookEventParams{
		EventID:     envelope.ID,
		SessionName: envelope.Session,
		EventType:   envelope.Event,
		Payload:     body,
	}); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "accepted", nil)
}
