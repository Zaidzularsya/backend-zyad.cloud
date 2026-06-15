package tenant

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidScope              = errors.New("invalid tenant repository scope")
	ErrOrganizationScopeMismatch = errors.New("organization does not match tenant repository scope")
)

// Scope is the immutable organization identity accepted by tenant-owned
// repositories. It can only be derived from a verified tenant context.
type Scope struct {
	organizationID string
	dataPlacement  DataPlacement
}

func NewScope(tenantContext Context) (Scope, error) {
	if !tenantContext.IsValid() ||
		strings.TrimSpace(tenantContext.OrganizationID()) == "" ||
		!tenantContext.DataPlacement().IsValid() {
		return Scope{}, ErrInvalidScope
	}
	return Scope{
		organizationID: tenantContext.OrganizationID(),
		dataPlacement:  tenantContext.DataPlacement(),
	}, nil
}

func RequireScope(ctx context.Context) (Scope, error) {
	tenantContext, err := RequireContext(ctx)
	if err != nil {
		return Scope{}, ErrMissingContext
	}
	return NewScope(tenantContext)
}

func (s Scope) OrganizationID() string {
	return s.organizationID
}

func (s Scope) DataPlacement() DataPlacement {
	return s.dataPlacement
}

func (s Scope) IsValid() bool {
	return strings.TrimSpace(s.organizationID) != "" && s.dataPlacement.IsValid()
}

func (s Scope) ValidateOrganization(organizationID string) error {
	if !s.IsValid() {
		return ErrInvalidScope
	}
	if strings.TrimSpace(organizationID) != s.organizationID {
		return ErrOrganizationScopeMismatch
	}
	return nil
}

func (s Scope) ValidateOrganizations(organizationIDs []string) error {
	if !s.IsValid() {
		return ErrInvalidScope
	}
	for _, organizationID := range organizationIDs {
		if err := s.ValidateOrganization(organizationID); err != nil {
			return err
		}
	}
	return nil
}
