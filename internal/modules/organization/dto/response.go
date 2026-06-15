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
	ExpiresAt     string         `json:"expires_at"`
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
