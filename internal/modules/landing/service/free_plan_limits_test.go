package service

import (
	"context"
	"testing"

	landingdomain "zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/organization"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

// hiddenFeatureGate behaves like EntitlementGuard for a plan that does not
// include the feature (Free: landing.custom_domain = false).
type hiddenFeatureGate struct{ calls []string }

func (g *hiddenFeatureGate) RequireFeature(_ context.Context, _ string, key string) (organizationmodel.Entitlement, error) {
	g.calls = append(g.calls, key)
	return organizationmodel.Entitlement{}, organization.FeatureNotEnabledError()
}

// capQuotaGuard behaves like EntitlementGuard.RequireQuotaValue with a fixed limit.
type capQuotaGuard struct{ limit int64 }

func (g capQuotaGuard) RequireQuotaValue(_ context.Context, _, _, _ string, used, delta int64) error {
	if used+delta > g.limit {
		return organization.QuotaExceededError()
	}
	return nil
}

// Regression: the backend (not only the UI) refuses a custom domain when the
// plan hides landing.custom_domain.
func TestCustomDomainRejectedWhenFeatureHidden(t *testing.T) {
	repo := &landingDomainRepoStub{availableDomains: []landingdomain.AvailableDomain{
		{ID: "domain-1", OrganizationID: landingServiceOrganizationID, Type: "custom"},
	}}
	svc := NewDomainService(repo, &landingDomainPageRepoStub{}, nil, WithLandingDomainFeatureGate(&hiddenFeatureGate{}))

	_, err := svc.BindDomain(context.Background(), mustLandingScope(t), BindDomainParams{OrganizationDomainID: "domain-1", LandingPageID: "page-1"})
	if err == nil || err.Error() != organization.FeatureNotEnabledError().Error() {
		t.Fatalf("BindDomain() error = %v, want FEATURE_NOT_ENABLED", err)
	}
	if repo.bindCalled {
		t.Fatal("custom domain must not be bound when the feature is hidden")
	}
}

// Regression: hiding landing.custom_domain must not block the platform subdomain.
func TestSubdomainBindNotAffectedByHiddenCustomDomainFeature(t *testing.T) {
	repo := &landingDomainRepoStub{availableDomains: []landingdomain.AvailableDomain{
		{ID: "domain-1", OrganizationID: landingServiceOrganizationID, Type: "subdomain"},
	}}
	gate := &hiddenFeatureGate{}
	svc := NewDomainService(repo, &landingDomainPageRepoStub{}, nil, WithLandingDomainFeatureGate(gate))

	if _, err := svc.BindDomain(context.Background(), mustLandingScope(t), BindDomainParams{OrganizationDomainID: "domain-1", LandingPageID: "page-1"}); err != nil {
		t.Fatalf("BindDomain() error = %v, want nil", err)
	}
	if len(gate.calls) != 0 || !repo.bindCalled {
		t.Fatalf("gate calls = %v bindCalled = %v", gate.calls, repo.bindCalled)
	}
}

// Regression: landing.max_pages = 3 admits the 3rd page and rejects the 4th.
func TestLandingPageCreateRejectedAtThree(t *testing.T) {
	create := func(existing int) error {
		svc := NewPageService(&pageServicePageRepoStub{listTotal: int64(existing)}, &pageServiceSectionRepoStub{}, WithLandingPageQuotaGuard(capQuotaGuard{limit: 3}))
		_, err := svc.Create(context.Background(), mustLandingScope(t), repository.CreatePageParams{
			Title: "My Page", Visibility: landingdomain.PageVisibilityPublic, Status: landingdomain.PageStatusDraft,
		})
		return err
	}
	if err := create(2); err != nil {
		t.Fatalf("3rd page: error = %v, want nil", err)
	}
	if err := create(3); !organization.IsQuotaExceeded(err) {
		t.Fatalf("4th page: error = %v, want QUOTA_EXCEEDED", err)
	}
}
