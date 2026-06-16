package service

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type OrganizationPermissionChecker interface {
	CanOrganization(
		context.Context,
		string,
		string,
		[]string,
	) error
}

type FeatureGate interface {
	RequireFeature(
		context.Context,
		string,
		string,
	) (organizationmodel.Entitlement, error)
}

type AccessPolicy struct {
	permissions OrganizationPermissionChecker
	features    FeatureGate
	featureKey  string
}

func NewAccessPolicy(
	permissions OrganizationPermissionChecker,
	features FeatureGate,
) *AccessPolicy {
	return &AccessPolicy{
		permissions: permissions,
		features:    features,
		featureKey:  landingdomain.FeatureLandingEnabled,
	}
}

func (p *AccessPolicy) AdminScope(
	ctx context.Context,
	tenantContext coretenant.Context,
	userID string,
	requiredPermissions ...string,
) (coretenant.Scope, error) {
	if err := requireActiveMembershipTenant(tenantContext); err != nil {
		return coretenant.Scope{}, err
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return coretenant.Scope{}, coreerrors.New(
			"UNAUTHORIZED",
			"user context is required",
			http.StatusUnauthorized,
		)
	}
	if p == nil || p.permissions == nil {
		return coretenant.Scope{}, coreerrors.New(
			"LANDING_PERMISSION_CHECKER_REQUIRED",
			"landing permission checker is required",
			http.StatusInternalServerError,
		)
	}
	if err := p.permissions.CanOrganization(
		ctx,
		userID,
		tenantContext.OrganizationID(),
		requiredPermissions,
	); err != nil {
		return coretenant.Scope{}, err
	}
	if err := p.requireLandingFeature(ctx, tenantContext.OrganizationID()); err != nil {
		return coretenant.Scope{}, err
	}
	return coretenant.NewScope(tenantContext)
}

func (p *AccessPolicy) PublishScope(
	ctx context.Context,
	tenantContext coretenant.Context,
	userID string,
) (coretenant.Scope, error) {
	return p.AdminScope(
		ctx,
		tenantContext,
		userID,
		landingdomain.PermissionPagePublish,
	)
}

func (p *AccessPolicy) PublicScope(
	ctx context.Context,
	tenantContext coretenant.Context,
) (coretenant.Scope, error) {
	if err := requireActiveTenant(tenantContext); err != nil {
		return coretenant.Scope{}, err
	}
	if err := p.requireLandingFeature(ctx, tenantContext.OrganizationID()); err != nil {
		return coretenant.Scope{}, err
	}
	return coretenant.NewScope(tenantContext)
}

func (p *AccessPolicy) requireLandingFeature(
	ctx context.Context,
	organizationID string,
) error {
	if p == nil || p.features == nil {
		return coreerrors.New(
			"LANDING_ENTITLEMENT_CHECKER_REQUIRED",
			"landing entitlement checker is required",
			http.StatusInternalServerError,
		)
	}
	_, err := p.features.RequireFeature(ctx, organizationID, p.featureKey)
	return err
}

func requireActiveMembershipTenant(tenantContext coretenant.Context) error {
	if err := requireActiveTenant(tenantContext); err != nil {
		return err
	}
	if tenantContext.MembershipID() == "" ||
		tenantContext.MembershipStatus() != "active" ||
		tenantContext.MembershipVersion() <= 0 {
		return coreerrors.New(
			"ACTIVE_MEMBERSHIP_REQUIRED",
			"active organization membership is required",
			http.StatusForbidden,
		)
	}
	return nil
}

func requireActiveTenant(tenantContext coretenant.Context) error {
	if !tenantContext.IsValid() {
		return coreerrors.New(
			"TENANT_CONTEXT_REQUIRED",
			"organization context is required",
			http.StatusForbidden,
		)
	}
	if !tenantContext.IsActive() {
		return coreerrors.New(
			"ORGANIZATION_NOT_ACTIVE",
			"organization is not active",
			http.StatusForbidden,
		)
	}
	return nil
}
