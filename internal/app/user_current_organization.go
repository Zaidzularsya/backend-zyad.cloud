package app

import (
	"context"

	organizationdto "zyad.cloud/internal/modules/organization/dto"
)

type userOrganizationLister interface {
	List(ctx context.Context, userID, sessionID string) ([]organizationdto.UserOrganizationResponse, error)
}

// userCurrentOrganizationProvider adapts the organization SwitchService to the user module's
// CurrentOrganizationProvider, so the user module never imports the organization module.
type userCurrentOrganizationProvider struct {
	organizations userOrganizationLister
}

// CurrentOrganization returns the entry flagged IsCurrent (same shape as /users/me/organizations),
// or nil when the session has no current organization.
func (p userCurrentOrganizationProvider) CurrentOrganization(ctx context.Context, userID, sessionID string) (any, error) {
	items, err := p.organizations.List(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.IsCurrent {
			return item, nil
		}
	}
	return nil, nil
}
