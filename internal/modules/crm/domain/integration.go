package domain

import "time"

type IntegrationProvider string

const (
	IntegrationProviderWebhook  IntegrationProvider = "webhook"
	IntegrationProviderWhatsApp IntegrationProvider = "whatsapp"
	IntegrationProviderEmail    IntegrationProvider = "email"
	IntegrationProviderZapier   IntegrationProvider = "zapier"
)

func (p IntegrationProvider) IsValid() bool {
	switch p {
	case IntegrationProviderWebhook, IntegrationProviderWhatsApp, IntegrationProviderEmail, IntegrationProviderZapier:
		return true
	default:
		return false
	}
}

// Integration's SecretEncrypted is opaque ciphertext (see
// internal/core/crypto.EncryptSecret) — nothing in this struct or its
// repository ever exposes plaintext. Only IntegrationService.RevealSecret
// decrypts it, gated by the integration.view_secret permission at the
// handler layer.
type Integration struct {
	ID              string
	OrganizationID  string
	Provider        IntegrationProvider
	Name            string
	Config          map[string]any
	SecretEncrypted string
	IsActive        bool
	ConnectedAt     *time.Time
	LastSyncedAt    *time.Time
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}
