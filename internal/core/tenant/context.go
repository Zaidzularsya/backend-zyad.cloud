// Package tenant defines the verified organization context shared across modules.
package tenant

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidContext = errors.New("invalid tenant context")
	ErrMissingContext = errors.New("tenant context is required")
)

type OrganizationType string

const (
	OrganizationTypePlatform OrganizationType = "platform"
	OrganizationTypeCustomer OrganizationType = "customer"
)

func (t OrganizationType) IsValid() bool {
	return t == OrganizationTypePlatform || t == OrganizationTypeCustomer
}

type OrganizationStatus string

const (
	OrganizationStatusPending            OrganizationStatus = "pending"
	OrganizationStatusActive             OrganizationStatus = "active"
	OrganizationStatusSuspended          OrganizationStatus = "suspended"
	OrganizationStatusDisabled           OrganizationStatus = "disabled"
	OrganizationStatusArchived           OrganizationStatus = "archived"
	OrganizationStatusProvisioning       OrganizationStatus = "provisioning"
	OrganizationStatusProvisioningFailed OrganizationStatus = "provisioning_failed"
)

func (s OrganizationStatus) IsValid() bool {
	switch s {
	case OrganizationStatusPending,
		OrganizationStatusActive,
		OrganizationStatusSuspended,
		OrganizationStatusDisabled,
		OrganizationStatusArchived,
		OrganizationStatusProvisioning,
		OrganizationStatusProvisioningFailed:
		return true
	default:
		return false
	}
}

type ResolutionSource string

const (
	ResolutionSourceSession      ResolutionSource = "session"
	ResolutionSourceHeader       ResolutionSource = "header"
	ResolutionSourceMembership   ResolutionSource = "membership_default"
	ResolutionSourcePlatformHost ResolutionSource = "platform_host"
	ResolutionSourceSubdomain    ResolutionSource = "subdomain"
	ResolutionSourceCustomDomain ResolutionSource = "custom_domain"
	ResolutionSourceWorker       ResolutionSource = "worker"
	ResolutionSourceInternal     ResolutionSource = "internal"
)

func (s ResolutionSource) IsValid() bool {
	switch s {
	case ResolutionSourceSession,
		ResolutionSourceHeader,
		ResolutionSourceMembership,
		ResolutionSourcePlatformHost,
		ResolutionSourceSubdomain,
		ResolutionSourceCustomDomain,
		ResolutionSourceWorker,
		ResolutionSourceInternal:
		return true
	default:
		return false
	}
}

type DataPlacement string

const (
	DataPlacementShared    DataPlacement = "shared"
	DataPlacementDedicated DataPlacement = "dedicated"
)

func (p DataPlacement) IsValid() bool {
	return p == DataPlacementShared || p == DataPlacementDedicated
}

type VerifiedContextInput struct {
	OrganizationID         string
	OrganizationSlug       string
	OrganizationType       OrganizationType
	OrganizationStatus     OrganizationStatus
	MembershipID           string
	MembershipStatus       string
	MembershipVersion      int64
	ResolutionSource       ResolutionSource
	DataPlacement          DataPlacement
	RequestHost            string
	IsPlatformOperator     bool
	ImpersonationSessionID string
}

// Context can only be created through NewVerifiedContext after a resolver has
// validated organization, membership, and resolution source.
type Context struct {
	organizationID         string
	organizationSlug       string
	organizationType       OrganizationType
	organizationStatus     OrganizationStatus
	membershipID           string
	membershipStatus       string
	membershipVersion      int64
	resolutionSource       ResolutionSource
	dataPlacement          DataPlacement
	requestHost            string
	isPlatformOperator     bool
	impersonationSessionID string
}

func NewVerifiedContext(input VerifiedContextInput) (Context, error) {
	input.OrganizationID = strings.TrimSpace(input.OrganizationID)
	input.OrganizationSlug = strings.TrimSpace(input.OrganizationSlug)
	input.MembershipID = strings.TrimSpace(input.MembershipID)
	input.MembershipStatus = strings.TrimSpace(input.MembershipStatus)
	input.RequestHost = strings.TrimSpace(input.RequestHost)
	input.ImpersonationSessionID = strings.TrimSpace(input.ImpersonationSessionID)

	if input.OrganizationID == "" ||
		input.OrganizationSlug == "" ||
		!input.OrganizationType.IsValid() ||
		!input.OrganizationStatus.IsValid() ||
		!input.ResolutionSource.IsValid() ||
		!input.DataPlacement.IsValid() {
		return Context{}, ErrInvalidContext
	}
	if requiresMembership(input.ResolutionSource) &&
		(input.MembershipID == "" ||
			input.MembershipStatus != "active" ||
			input.MembershipVersion <= 0) {
		return Context{}, ErrInvalidContext
	}

	return Context{
		organizationID:         input.OrganizationID,
		organizationSlug:       input.OrganizationSlug,
		organizationType:       input.OrganizationType,
		organizationStatus:     input.OrganizationStatus,
		membershipID:           input.MembershipID,
		membershipStatus:       input.MembershipStatus,
		membershipVersion:      input.MembershipVersion,
		resolutionSource:       input.ResolutionSource,
		dataPlacement:          input.DataPlacement,
		requestHost:            input.RequestHost,
		isPlatformOperator:     input.IsPlatformOperator,
		impersonationSessionID: input.ImpersonationSessionID,
	}, nil
}

func (c Context) OrganizationID() string             { return c.organizationID }
func (c Context) OrganizationSlug() string           { return c.organizationSlug }
func (c Context) OrganizationType() OrganizationType { return c.organizationType }
func (c Context) OrganizationStatus() OrganizationStatus {
	return c.organizationStatus
}
func (c Context) MembershipID() string               { return c.membershipID }
func (c Context) MembershipStatus() string           { return c.membershipStatus }
func (c Context) MembershipVersion() int64           { return c.membershipVersion }
func (c Context) ResolutionSource() ResolutionSource { return c.resolutionSource }
func (c Context) DataPlacement() DataPlacement       { return c.dataPlacement }
func (c Context) RequestHost() string                { return c.requestHost }
func (c Context) IsPlatformOperator() bool           { return c.isPlatformOperator }
func (c Context) ImpersonationSessionID() string     { return c.impersonationSessionID }

func (c Context) IsValid() bool {
	return c.organizationID != "" &&
		c.organizationSlug != "" &&
		c.organizationType.IsValid() &&
		c.organizationStatus.IsValid() &&
		c.resolutionSource.IsValid() &&
		c.dataPlacement.IsValid()
}

func (c Context) IsActive() bool {
	return c.IsValid() && c.organizationStatus == OrganizationStatusActive
}

type contextKey struct{}

func WithContext(ctx context.Context, tenantContext Context) context.Context {
	if !tenantContext.IsValid() {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, tenantContext)
}

func FromContext(ctx context.Context) (Context, bool) {
	tenantContext, ok := ctx.Value(contextKey{}).(Context)
	return tenantContext, ok && tenantContext.IsValid()
}

func RequireContext(ctx context.Context) (Context, error) {
	tenantContext, ok := FromContext(ctx)
	if !ok {
		return Context{}, ErrMissingContext
	}
	return tenantContext, nil
}

func requiresMembership(source ResolutionSource) bool {
	return source == ResolutionSourceSession ||
		source == ResolutionSourceHeader ||
		source == ResolutionSourceMembership
}
