package dto

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type OrganizationResponse struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Slug          string         `json:"slug"`
	Name          string         `json:"name"`
	Status        string         `json:"status"`
	Timezone      string         `json:"timezone"`
	Locale        string         `json:"locale"`
	Region        string         `json:"region,omitempty"`
	DataPlacement string         `json:"data_placement"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	DeletedAt     *string        `json:"deleted_at,omitempty"`
}

type OrganizationListResponse struct {
	Items []OrganizationResponse `json:"items"`
	Meta  PaginationMeta         `json:"meta"`
}

type OrganizationPlacementSummary struct {
	Type      string `json:"type"`
	Supported bool   `json:"supported"`
	Status    string `json:"status"`
}

type OrganizationDomainSummary struct {
	Total         int64 `json:"total"`
	Active        int64 `json:"active"`
	Pending       int64 `json:"pending"`
	Failed        int64 `json:"failed"`
	SSLFailed     int64 `json:"ssl_failed"`
	PrimaryActive int64 `json:"primary_active"`
}

type OrganizationEntitlementSummary struct {
	Total      int64 `json:"total"`
	Active     int64 `json:"active"`
	MaxVersion int64 `json:"max_version"`
}

type OrganizationHealthSummary struct {
	Status string   `json:"status"`
	Issues []string `json:"issues"`
}

type PlatformOrganizationDetailResponse struct {
	Organization OrganizationResponse           `json:"organization"`
	Placement    OrganizationPlacementSummary   `json:"placement"`
	Domains      OrganizationDomainSummary      `json:"domains"`
	Entitlements OrganizationEntitlementSummary `json:"entitlements"`
	Health       OrganizationHealthSummary      `json:"health"`
}

type MembershipResponse struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	UserID         string   `json:"user_id"`
	UserName       string   `json:"user_name,omitempty"`
	UserEmail      string   `json:"user_email,omitempty"`
	Status         string   `json:"status"`
	IsOwner        bool     `json:"is_owner"`
	Version        int64    `json:"version"`
	RoleIDs        []string `json:"role_ids"`
	RoleSlugs      []string `json:"role_slugs"`
	InvitedBy      *string  `json:"invited_by,omitempty"`
	InvitedEmail   string   `json:"invited_email,omitempty"`
	InvitedAt      *string  `json:"invited_at,omitempty"`
	AcceptedAt     *string  `json:"accepted_at,omitempty"`
	SuspendedAt    *string  `json:"suspended_at,omitempty"`
	RemovedAt      *string  `json:"removed_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

type InvitationResponse struct {
	Membership      MembershipResponse `json:"membership"`
	InvitationToken string             `json:"invitation_token"`
	ExpiresAt       string             `json:"expires_at"`
}

type UserOrganizationResponse struct {
	Organization OrganizationResponse `json:"organization"`
	Membership   MembershipResponse   `json:"membership"`
	IsCurrent    bool                 `json:"is_current"`
}

type SwitchOrganizationResponse struct {
	CurrentOrganization UserOrganizationResponse `json:"current_organization"`
	AccessToken         string                   `json:"access_token,omitempty"`
	RefreshToken        string                   `json:"refresh_token,omitempty"`
	TokenType           string                   `json:"token_type,omitempty"`
	ExpiresIn           int64                    `json:"expires_in,omitempty"`
}

type DomainResponse struct {
	ID                   string  `json:"id"`
	OrganizationID       string  `json:"organization_id"`
	Type                 string  `json:"type"`
	CanonicalHost        string  `json:"canonical_host"`
	Status               string  `json:"status"`
	IsPrimary            bool    `json:"is_primary"`
	VerificationAttempts int     `json:"verification_attempts"`
	LastVerificationAt   *string `json:"last_verification_at,omitempty"`
	VerifiedAt           *string `json:"verified_at,omitempty"`
	VerificationError    string  `json:"verification_error,omitempty"`
	SSLStatus            string  `json:"ssl_status"`
	SSLError             string  `json:"ssl_error,omitempty"`
	SSLExpiresAt         *string `json:"ssl_expires_at,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type DomainChallengeResponse struct {
	Domain        DomainResponse `json:"domain"`
	ChallengeType string         `json:"challenge_type"`
	RecordName    string         `json:"record_name"`
	RecordValue   string         `json:"record_value"`
}

type EntitlementResponse struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organization_id"`
	FeatureKey      string         `json:"feature_key"`
	Source          string         `json:"source"`
	SourceReference string         `json:"source_reference,omitempty"`
	Status          string         `json:"status"`
	Limits          map[string]any `json:"limits"`
	Version         int64          `json:"version"`
	EffectiveFrom   string         `json:"effective_from"`
	EffectiveUntil  *string        `json:"effective_until,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

type EffectiveFeatureResponse struct {
	FeatureKey     string         `json:"feature_key"`
	Enabled        bool           `json:"enabled"`
	Limits         map[string]any `json:"limits"`
	EffectiveUntil *string        `json:"effective_until,omitempty"`
	Version        int64          `json:"version"`
}

type EffectiveFeatureListResponse struct {
	Items []EffectiveFeatureResponse `json:"items"`
	Meta  PaginationMeta             `json:"meta"`
}

type UsageResponse struct {
	FeatureKey     string  `json:"feature_key"`
	MetricKey      string  `json:"metric_key"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	UsageValue     int64   `json:"usage_value"`
	LimitValue     *int64  `json:"limit_value,omitempty"`
	RemainingValue *int64  `json:"remaining_value,omitempty"`
	Version        int64   `json:"version"`
	LastRecordedAt *string `json:"last_recorded_at,omitempty"`
}

type ImpersonationSessionResponse struct {
	ID                   string         `json:"id"`
	OperatorSessionID    string         `json:"operator_session_id"`
	OperatorUserID       string         `json:"operator_user_id"`
	TargetOrganizationID string         `json:"target_organization_id"`
	TargetUserID         string         `json:"target_user_id,omitempty"`
	Reason               string         `json:"reason"`
	TicketReference      string         `json:"ticket_reference,omitempty"`
	StartedAt            string         `json:"started_at"`
	ExpiresAt            string         `json:"expires_at"`
	StoppedAt            *string        `json:"stopped_at,omitempty"`
	StoppedByUserID      string         `json:"stopped_by_user_id,omitempty"`
	StopReason           string         `json:"stop_reason,omitempty"`
	Metadata             map[string]any `json:"metadata"`
	IsActive             bool           `json:"is_active"`
}
