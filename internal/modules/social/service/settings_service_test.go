package service

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/social/domain"
)

type fakeSettingsStore struct {
	rows map[string]domain.TenantSettings
}

func (f *fakeSettingsStore) Get(_ context.Context, orgID string) (domain.TenantSettings, bool, error) {
	s, ok := f.rows[orgID]
	return s, ok, nil
}

func (f *fakeSettingsStore) Upsert(_ context.Context, s domain.TenantSettings) (domain.TenantSettings, error) {
	now := time.Now().UTC()
	s.UpdatedAt = &now
	f.rows[s.OrganizationID] = s
	return s, nil
}

func TestSettingsDefaultToAutoOfferOn(t *testing.T) {
	svc := NewSettingsService(&fakeSettingsStore{rows: map[string]domain.TenantSettings{}})
	got, err := svc.Get(context.Background(), "org-a")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CustomerClaimAutoOffer || got.OrganizationID != "org-a" || got.UpdatedAt != nil {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestSettingsSetAutoOffer(t *testing.T) {
	store := &fakeSettingsStore{rows: map[string]domain.TenantSettings{}}
	svc := NewSettingsService(store)
	if _, err := svc.SetAutoOffer(context.Background(), "org-a", false, "user-1"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background(), "org-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomerClaimAutoOffer || got.UpdatedBy != "user-1" || got.UpdatedAt == nil {
		t.Fatalf("after update = %+v", got)
	}
}
