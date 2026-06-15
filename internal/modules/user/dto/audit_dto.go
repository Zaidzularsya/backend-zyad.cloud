package dto

type LoginHistoryResponse struct {
	ID         string  `json:"id"`
	UserID     *string `json:"user_id,omitempty"`
	Identifier string  `json:"identifier"`
	Event      string  `json:"event"`
	Success    bool    `json:"success"`
	IPAddress  string  `json:"ip_address"`
	UserAgent  string  `json:"user_agent"`
	DeviceName string  `json:"device_name"`
	Reason     string  `json:"reason"`
	CreatedAt  string  `json:"created_at"`
}

type LoginHistoryQuery struct {
	Page        int    `form:"page"`
	PerPage     int    `form:"per_page"`
	UserID      string `form:"user_id"`
	Event       string `form:"event"`
	Success     *bool  `form:"success"`
	IPAddress   string `form:"ip_address"`
	Search      string `form:"search"`
	CreatedFrom string `form:"created_from"`
	CreatedTo   string `form:"created_to"`
	Sort        string `form:"sort"`
	Direction   string `form:"direction"`
}

type AuditLogResponse struct {
	ID                     string         `json:"id"`
	Module                 string         `json:"module"`
	Event                  string         `json:"event"`
	OrganizationID         *string        `json:"organization_id,omitempty"`
	MembershipID           *string        `json:"membership_id,omitempty"`
	SessionID              *string        `json:"session_id,omitempty"`
	ActorUserID            *string        `json:"actor_user_id,omitempty"`
	OperatorUserID         *string        `json:"operator_user_id,omitempty"`
	EffectiveUserID        *string        `json:"effective_user_id,omitempty"`
	ImpersonationSessionID *string        `json:"impersonation_session_id,omitempty"`
	ResolutionSource       string         `json:"resolution_source,omitempty"`
	RequestID              string         `json:"request_id,omitempty"`
	TargetUserID           *string        `json:"target_user_id,omitempty"`
	TargetType             string         `json:"target_type"`
	TargetID               *string        `json:"target_id,omitempty"`
	Metadata               map[string]any `json:"metadata"`
	IPAddress              string         `json:"ip_address"`
	UserAgent              string         `json:"user_agent"`
	CreatedAt              string         `json:"created_at"`
}

type AuditLogQuery struct {
	Page         int    `form:"page"`
	PerPage      int    `form:"per_page"`
	OrganizationID string `form:"organization_id"`
	Module       string `form:"module"`
	Event        string `form:"event"`
	ActorUserID  string `form:"actor_user_id"`
	TargetUserID string `form:"target_user_id"`
	Search       string `form:"search"`
	CreatedFrom  string `form:"created_from"`
	CreatedTo    string `form:"created_to"`
	Sort         string `form:"sort"`
	Direction    string `form:"direction"`
}
