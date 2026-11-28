package service

import (
	"context"
	"fmt"
	"strings"

	"zyad.cloud/internal/modules/social/domain"
)

// SettingsStore persists social_tenant_settings. The table is global (no RLS):
// every call is keyed by an organization id taken from the verified tenant scope.
type SettingsStore interface {
	// Get returns the row, found=false when the tenant has none yet.
	Get(ctx context.Context, organizationID string) (domain.TenantSettings, bool, error)
	Upsert(ctx context.Context, settings domain.TenantSettings) (domain.TenantSettings, error)
}

type SettingsService struct {
	store SettingsStore
}

func NewSettingsService(store SettingsStore) *SettingsService {
	return &SettingsService{store: store}
}

// Get returns the tenant's settings, or the defaults when no row exists.
func (s *SettingsService) Get(ctx context.Context, organizationID string) (domain.TenantSettings, error) {
	organizationID = strings.TrimSpace(organizationID)
	settings, found, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return domain.TenantSettings{}, fmt.Errorf("load social tenant settings: %w", err)
	}
	if !found {
		return domain.DefaultTenantSettings(organizationID), nil
	}
	return settings, nil
}

// SetAutoOffer turns automatic claim offers for the tenant's contacts on or off.
func (s *SettingsService) SetAutoOffer(ctx context.Context, organizationID string, autoOffer bool, actorUserID string) (domain.TenantSettings, error) {
	return s.store.Upsert(ctx, domain.TenantSettings{
		OrganizationID:         strings.TrimSpace(organizationID),
		CustomerClaimAutoOffer: autoOffer,
		UpdatedBy:              strings.TrimSpace(actorUserID),
	})
}
