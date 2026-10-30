package app

import (
	"context"
	"testing"

	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationdto "zyad.cloud/internal/modules/organization/dto"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	userservice "zyad.cloud/internal/modules/user/service"
)

type fakeOwnedCreator struct {
	userID string
	req    organizationdto.CreateWorkspaceRequest
	meta   organizationservice.OnboardingMetadata
}

func (f *fakeOwnedCreator) CreateOwnedWorkspace(_ context.Context, userID string, req organizationdto.CreateWorkspaceRequest, meta organizationservice.OnboardingMetadata) (string, error) {
	f.userID, f.req, f.meta = userID, req, meta
	return "ws-new", nil
}

func TestTenantBuyerWorkspacesCreateMarksTenantAndUsesUniqueSlug(t *testing.T) {
	creator := &fakeOwnedCreator{}
	w := tenantBuyerWorkspaces{onboarding: creator}
	buyer := crmservice.TenantBuyer{UserID: "0f0e0d0c-0000-4000-8000-000000000001", Name: "Rina Wati", Email: "rina@example.com"}

	id, err := w.CreateForTenant(context.Background(), "tenant-a", buyer, "203.0.113.1", "ua")
	if err != nil || id != "ws-new" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	if creator.userID != buyer.UserID || creator.req.Name != "Rina Wati" || creator.meta.IPAddress != "203.0.113.1" {
		t.Fatalf("creator got user=%s req=%+v meta=%+v", creator.userID, creator.req, creator.meta)
	}
	if creator.req.Metadata[tenantCheckoutMetadataKey] != "tenant-a" {
		t.Fatalf("metadata = %v", creator.req.Metadata)
	}
	first := creator.req.Slug
	if _, err := w.CreateForTenant(context.Background(), "tenant-b", buyer, "", ""); err != nil {
		t.Fatal(err)
	}
	if first == creator.req.Slug {
		t.Fatalf("one buyer, two tenants: slugs must differ, both %q", first)
	}
}

func TestTenantBuyerWorkspaceRequestFallsBackToEmail(t *testing.T) {
	req, err := tenantBuyerWorkspaceRequest("tenant-a", crmservice.TenantBuyer{UserID: "u", Email: "x@example.com"})
	if err != nil || req.Name != "x@example.com" {
		t.Fatalf("req=%+v err=%v", req, err)
	}
}

type fakeAuthBuyer struct {
	meta userservice.LoginHistoryRecord
}

func (f *fakeAuthBuyer) ResolveGoogleBuyer(_ context.Context, _ string, meta userservice.LoginHistoryRecord) (userservice.GoogleBuyer, error) {
	f.meta = meta
	return userservice.GoogleBuyer{UserID: "u1", Name: "N", Email: "e@example.com", IsNewUser: true}, nil
}

func TestGoogleBuyerAdapterPassesMetadata(t *testing.T) {
	auth := &fakeAuthBuyer{}
	b, err := googleBuyerAdapter{auth: auth}.ResolveGoogleBuyer(context.Background(), "cred", "203.0.113.2", "ua")
	if err != nil || b.UserID != "u1" || b.Email != "e@example.com" {
		t.Fatalf("b=%+v err=%v", b, err)
	}
	if auth.meta.IPAddress != "203.0.113.2" || auth.meta.UserAgent != "ua" {
		t.Fatalf("meta=%+v", auth.meta)
	}
}
