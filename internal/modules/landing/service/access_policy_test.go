package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	coreerrors "zyad.cloud/internal/core/errors"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

const (
	accessPolicyOrganizationID = "11111111-1111-1111-1111-111111111111"
	accessPolicyOtherOrgID     = "22222222-2222-2222-2222-222222222222"
	accessPolicyUserID         = "33333333-3333-3333-3333-333333333333"
)

type accessPolicyPermissionChecker struct {
	organizationID string
	permissions    []string
	err            error
}

func (c *accessPolicyPermissionChecker) CanOrganization(
	_ context.Context,
	_ string,
	organizationID string,
	permissions []string,
) error {
	c.organizationID = organizationID
	c.permissions = append(c.permissions, permissions...)
	return c.err
}

type accessPolicyFeatureGate struct {
	organizationID string
	featureKey     string
	err            error
}

func (g *accessPolicyFeatureGate) RequireFeature(
	_ context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	g.organizationID = organizationID
	g.featureKey = featureKey
	return organizationmodel.Entitlement{
		OrganizationID: organizationID,
		FeatureKey:     featureKey,
	}, g.err
}

func TestAccessPolicyAdminScopeRequiresPermissionAndEntitlement(t *testing.T) {
	permissions := &accessPolicyPermissionChecker{}
	features := &accessPolicyFeatureGate{}
	policy := NewAccessPolicy(permissions, features)

	scope, err := policy.AdminScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.ResolutionSourceSession,
		),
		accessPolicyUserID,
		landingdomain.PermissionPageRead,
	)
	if err != nil {
		t.Fatalf("AdminScope() error = %v", err)
	}
	if scope.OrganizationID() != accessPolicyOrganizationID {
		t.Fatalf("scope organization = %q", scope.OrganizationID())
	}
	if permissions.organizationID != accessPolicyOrganizationID ||
		len(permissions.permissions) != 1 ||
		permissions.permissions[0] != landingdomain.PermissionPageRead {
		t.Fatalf("permissions=%#v", permissions)
	}
	if features.organizationID != accessPolicyOrganizationID ||
		features.featureKey != landingdomain.FeatureLandingEnabled {
		t.Fatalf("features=%#v", features)
	}
}

func TestAccessPolicyPublishScopeUsesPublishPermission(t *testing.T) {
	permissions := &accessPolicyPermissionChecker{}
	policy := NewAccessPolicy(permissions, &accessPolicyFeatureGate{})

	_, err := policy.PublishScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.ResolutionSourceSession,
		),
		accessPolicyUserID,
	)
	if err != nil {
		t.Fatalf("PublishScope() error = %v", err)
	}
	if len(permissions.permissions) != 1 ||
		permissions.permissions[0] != landingdomain.PermissionPagePublish {
		t.Fatalf("permissions=%#v", permissions.permissions)
	}
}

func TestAccessPolicyAdminScopeRejectsPublicHostContext(t *testing.T) {
	policy := NewAccessPolicy(
		&accessPolicyPermissionChecker{},
		&accessPolicyFeatureGate{},
	)

	_, err := policy.AdminScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypePlatform,
			coretenant.ResolutionSourcePlatformHost,
		),
		accessPolicyUserID,
		landingdomain.PermissionPageRead,
	)
	if err == nil {
		t.Fatal("AdminScope() expected error for public host context")
	}
}

func TestAccessPolicyPublicScopeAcceptsPlatformHostContext(t *testing.T) {
	features := &accessPolicyFeatureGate{}
	policy := NewAccessPolicy(nil, features)

	scope, err := policy.PublicScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypePlatform,
			coretenant.ResolutionSourcePlatformHost,
		),
	)
	if err != nil {
		t.Fatalf("PublicScope() error = %v", err)
	}
	if scope.OrganizationID() != accessPolicyOrganizationID ||
		features.featureKey != landingdomain.FeatureLandingEnabled {
		t.Fatalf("scope=%#v features=%#v", scope, features)
	}
}

func TestAccessPolicyAdminScopePropagatesSuspendedSubscription(t *testing.T) {
	permissions := &accessPolicyPermissionChecker{}
	features := &accessPolicyFeatureGate{
		err: coreerrors.New(
			"SUBSCRIPTION_SUSPENDED",
			"billing subscription is suspended",
			403,
		),
	}
	policy := NewAccessPolicy(permissions, features)

	_, err := policy.AdminScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.ResolutionSourceSession,
		),
		accessPolicyUserID,
		landingdomain.PermissionPageRead,
	)
	if err == nil {
		t.Fatal("AdminScope() expected suspended subscription error")
	}
	appErr, ok := err.(*coreerrors.AppError)
	if !ok {
		t.Fatalf("error type = %T, want *AppError", err)
	}
	if appErr.Code != "SUBSCRIPTION_SUSPENDED" {
		t.Fatalf("error code = %s, want SUBSCRIPTION_SUSPENDED", appErr.Code)
	}
}

func TestAccessPolicyScopeRejectsCrossTenantRepositoryLookup(t *testing.T) {
	policy := NewAccessPolicy(
		&accessPolicyPermissionChecker{},
		&accessPolicyFeatureGate{},
	)
	scope, err := policy.AdminScope(
		context.Background(),
		accessPolicyTenantContext(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.ResolutionSourceSession,
		),
		accessPolicyUserID,
		landingdomain.PermissionPageRead,
	)
	if err != nil {
		t.Fatalf("AdminScope() error = %v", err)
	}
	if err := scope.ValidateOrganization(accessPolicyOtherOrgID); !errors.Is(
		err,
		coretenant.ErrOrganizationScopeMismatch,
	) {
		t.Fatalf("ValidateOrganization() error = %v", err)
	}
}

func accessPolicyTenantContext(
	t *testing.T,
	organizationType coretenant.OrganizationType,
	resolutionSource coretenant.ResolutionSource,
) coretenant.Context {
	t.Helper()
	input := coretenant.VerifiedContextInput{
		OrganizationID:     accessPolicyOrganizationID,
		OrganizationSlug:   "acme",
		OrganizationType:   organizationType,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   resolutionSource,
		DataPlacement:      coretenant.DataPlacementShared,
		RequestHost:        "landing.example.test",
	}
	if resolutionSource == coretenant.ResolutionSourceSession ||
		resolutionSource == coretenant.ResolutionSourceHeader ||
		resolutionSource == coretenant.ResolutionSourceMembership {
		input.MembershipID = "44444444-4444-4444-4444-444444444444"
		input.MembershipStatus = "active"
		input.MembershipVersion = 1
	}
	tenantContext, err := coretenant.NewVerifiedContext(input)
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
