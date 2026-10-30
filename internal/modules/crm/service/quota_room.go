package service

import "context"

// QuotaRoomRequirer is the narrow consumer-side view of
// organization/service.QuotaEnforcer: it fails with QUOTA_EXCEEDED /
// FEATURE_NOT_ENABLED when the organization has no room for one more unit of
// featureKey. It is shared by the lead, company, deal and pipeline services.
//
// It must only be called on creation paths. Update/Get/List never consult it
// so tenants already over their limit keep read and edit access.
type QuotaRoomRequirer interface {
	RequireRoom(ctx context.Context, organizationID, featureKey string) error
}
