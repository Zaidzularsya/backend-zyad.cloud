package app

import (
	"log/slog"
	"strings"
	"time"

	"zyad.cloud/internal/config"
	whatsapprepo "zyad.cloud/internal/modules/whatsapp/repository"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

// NewWAHAProvider returns the WAHA client when WHATSAPP_PROVIDER=waha and
// credentials are set, otherwise nil (the session service then answers
// provider actions with WHATSAPP_NOT_CONFIGURED). Shared by cmd/api and
// cmd/worker.
func NewWAHAProvider(cfg config.WhatsAppConfig) *platformwhatsapp.WAHAClient {
	wahaConfig := platformwhatsapp.WAHAConfig{
		BaseURL:        cfg.URL,
		APIKey:         cfg.APIKey,
		DefaultSession: cfg.Session,
		Timeout:        time.Duration(cfg.TimeoutSeconds) * time.Second,
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Provider), platformwhatsapp.ProviderWAHA) || !wahaConfig.HasCredentials() {
		return nil
	}
	return platformwhatsapp.NewWAHAClient(wahaConfig)
}

// NewWhatsAppSessionService wires the session service. quota may be nil
// (worker: reconcile does not create sessions).
func NewWhatsAppSessionService(
	cfg config.Config,
	db *database.Pool,
	quota whatsappservice.SessionQuotaGuard,
	log *slog.Logger,
) *whatsappservice.SessionService {
	var provider whatsappservice.SessionProvider
	if client := NewWAHAProvider(cfg.WhatsApp); client != nil {
		provider = client
	}
	options := []whatsappservice.SessionServiceOption{whatsappservice.WithSessionLogger(log)}
	if quota != nil {
		options = append(options, whatsappservice.WithSessionQuotaGuard(quota))
	}
	return whatsappservice.NewSessionService(
		whatsapprepo.NewSessionRepository(db),
		provider,
		whatsappservice.SessionServiceConfig{
			WebhookURL:     cfg.WhatsApp.CallbackURL,
			WebhookHMACKey: cfg.WhatsApp.WebhookHMACKey,
			Engine:         cfg.WhatsApp.Engine,
		},
		options...,
	)
}
