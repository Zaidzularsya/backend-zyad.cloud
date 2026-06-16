package dto

type OrganizationListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Type           string `form:"type"`
	Status         string `form:"status"`
	DataPlacement  string `form:"data_placement"`
	Search         string `form:"search"`
	IncludeDeleted bool   `form:"include_deleted"`
	Sort           string `form:"sort"`
	Direction      string `form:"direction"`
}

type CreateOrganizationRequest struct {
	Type          string         `json:"type" binding:"required"`
	Slug          string         `json:"slug" binding:"required"`
	Name          string         `json:"name" binding:"required"`
	Timezone      string         `json:"timezone"`
	Locale        string         `json:"locale"`
	Region        string         `json:"region"`
	DataPlacement string         `json:"data_placement"`
	OwnerUserID   string         `json:"owner_user_id" binding:"required"`
	Metadata      map[string]any `json:"metadata"`
}

type UpdateOrganizationRequest struct {
	Slug          *string         `json:"slug"`
	Name          *string         `json:"name"`
	Timezone      *string         `json:"timezone"`
	Locale        *string         `json:"locale"`
	Region        *string         `json:"region"`
	DataPlacement *string         `json:"data_placement"`
	Metadata      *map[string]any `json:"metadata"`
}

type UpdateOrganizationStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

type UpdateCurrentOrganizationRequest struct {
	Name     *string         `json:"name"`
	Timezone *string         `json:"timezone"`
	Locale   *string         `json:"locale"`
	Region   *string         `json:"region"`
	Metadata *map[string]any `json:"metadata"`
}

type InviteMemberRequest struct {
	Email     string   `json:"email" binding:"required,email"`
	RoleIDs   []string `json:"role_ids"`
	ExpiresAt *string  `json:"expires_at"`
}

type MembershipListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Status         string `form:"status"`
	IncludeRemoved bool   `form:"include_removed"`
}

type UpdateMembershipStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason"`
}

type RemoveMembershipRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type TransferOwnershipRequest struct {
	MembershipID string `json:"membership_id" binding:"required"`
}

type SwitchOrganizationRequest struct {
	OrganizationID string `json:"organization_id" binding:"required"`
}

type CreateDomainRequest struct {
	Type          string `json:"type" binding:"required"`
	CanonicalHost string `json:"canonical_host" binding:"required"`
	IsPrimary     bool   `json:"is_primary"`
}

type DomainListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Type           string `form:"type"`
	Status         string `form:"status"`
	IncludeDeleted bool   `form:"include_deleted"`
}

type UpdateDomainRequest struct {
	IsPrimary *bool `json:"is_primary"`
}

type UpsertEntitlementRequest struct {
	FeatureKey      string         `json:"feature_key" binding:"required"`
	Source          string         `json:"source" binding:"required"`
	SourceReference string         `json:"source_reference"`
	Status          string         `json:"status"`
	Limits          map[string]any `json:"limits"`
	EffectiveFrom   *string        `json:"effective_from"`
	EffectiveUntil  *string        `json:"effective_until"`
	Reason          string         `json:"reason"`
}

type EntitlementListQuery struct {
	Page       int    `form:"page"`
	PerPage    int    `form:"per_page"`
	FeatureKey string `form:"feature_key"`
}

type UsageQuery struct {
	FeatureKey  string `form:"feature_key" binding:"required"`
	MetricKey   string `form:"metric_key" binding:"required"`
	LimitKey    string `form:"limit_key" binding:"required"`
	PeriodStart string `form:"period_start" binding:"required"`
	PeriodEnd   string `form:"period_end" binding:"required"`
}

type StartImpersonationRequest struct {
	TargetUserID    string         `json:"target_user_id"`
	Reason          string         `json:"reason" binding:"required"`
	TicketReference string         `json:"ticket_reference"`
	ExpiresAt       string         `json:"expires_at" binding:"required"`
	Metadata        map[string]any `json:"metadata"`
}

type StopImpersonationRequest struct {
	Reason string `json:"reason" binding:"required"`
}
