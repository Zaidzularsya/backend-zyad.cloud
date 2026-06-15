package dto

type UserListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Search         string `form:"search"`
	Status         string `form:"status"`
	Role           string `form:"role"`
	OrganizationID string `form:"organization_id"`
	IncludeDeleted bool   `form:"include_deleted"`
	CreatedFrom    string `form:"created_from"`
	CreatedTo      string `form:"created_to"`
	Sort           string `form:"sort"`
	Direction      string `form:"direction"`
}

type UserDetailQuery struct {
	IncludeDeleted bool `form:"include_deleted"`
}

type CreateUserRequest struct {
	Name           string                   `json:"name" binding:"required"`
	Email          string                   `json:"email" binding:"required,email"`
	Username       string                   `json:"username"`
	Phone          string                   `json:"phone"`
	Password       string                   `json:"password"`
	Status         string                   `json:"status"`
	Profile        CreateUserProfileRequest `json:"profile"`
	Roles          []CreateUserRoleRequest  `json:"roles"`
	SendInvitation bool                     `json:"send_invitation"`
}

type CreateUserProfileRequest struct {
	AvatarURL  string `json:"avatar_url"`
	Bio        string `json:"bio"`
	JobTitle   string `json:"job_title"`
	Department string `json:"department"`
	Company    string `json:"company"`
	Address    string `json:"address"`
	Timezone   string `json:"timezone"`
	Language   string `json:"language"`
}

type CreateUserRoleRequest struct {
	RoleID         string  `json:"role_id" binding:"required"`
	OrganizationID *string `json:"organization_id"`
}

type UpdateUserRequest struct {
	Name       *string `json:"name"`
	Email      *string `json:"email"`
	Username   *string `json:"username"`
	Phone      *string `json:"phone"`
	Status     *string `json:"status"`
	AvatarURL  *string `json:"avatar_url"`
	Bio        *string `json:"bio"`
	JobTitle   *string `json:"job_title"`
	Department *string `json:"department"`
	Company    *string `json:"company"`
	Address    *string `json:"address"`
	Timezone   *string `json:"timezone"`
	Language   *string `json:"language"`
}

type BulkUserActionRequest struct {
	Action  string                `json:"action" binding:"required"`
	UserIDs []string              `json:"user_ids" binding:"required"`
	Payload BulkUserActionPayload `json:"payload"`
}

type BulkUserActionPayload struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type BulkUserActionResponse struct {
	Action    string                 `json:"action"`
	Total     int                    `json:"total"`
	Succeeded int                    `json:"succeeded"`
	Failed    int                    `json:"failed"`
	Results   []BulkUserActionResult `json:"results"`
}

type BulkUserActionResult struct {
	UserID  string               `json:"user_id"`
	Success bool                 `json:"success"`
	Error   *BulkUserActionError `json:"error,omitempty"`
}

type BulkUserActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason"`
}

type UserStatusReasonRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type UserListItem struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Email           string   `json:"email"`
	Username        string   `json:"username,omitempty"`
	Phone           string   `json:"phone,omitempty"`
	Status          string   `json:"status"`
	EmailVerifiedAt *string  `json:"email_verified_at"`
	PhoneVerifiedAt *string  `json:"phone_verified_at"`
	LastLoginAt     *string  `json:"last_login_at"`
	Roles           []string `json:"roles"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	DeletedAt       *string  `json:"deleted_at"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type UserProfileDetail struct {
	AvatarURL  string `json:"avatar_url,omitempty"`
	Bio        string `json:"bio,omitempty"`
	JobTitle   string `json:"job_title,omitempty"`
	Department string `json:"department,omitempty"`
	Company    string `json:"company,omitempty"`
	Address    string `json:"address,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Language   string `json:"language,omitempty"`
}

type UserRoleDetail struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	OrganizationID string `json:"organization_id,omitempty"`
	AssignedAt     string `json:"assigned_at"`
}

type UserPermissionDetail struct {
	ID             string `json:"id"`
	PermissionID   string `json:"permission_id"`
	Slug           string `json:"slug"`
	Effect         string `json:"effect"`
	OrganizationID string `json:"organization_id,omitempty"`
	AssignedAt     string `json:"assigned_at"`
}

type UserSessionSummary struct {
	Total        int64   `json:"total"`
	Active       int64   `json:"active"`
	Revoked      int64   `json:"revoked"`
	Expired      int64   `json:"expired"`
	LastActiveAt *string `json:"last_active_at"`
}

type UserAuditSummary struct {
	Total         int64   `json:"total"`
	LastEvent     string  `json:"last_event,omitempty"`
	LastEventAt   *string `json:"last_event_at"`
	LastActorID   string  `json:"last_actor_id,omitempty"`
	LastIPAddress string  `json:"last_ip_address,omitempty"`
}

type UserDetailResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Email           string                 `json:"email"`
	Username        string                 `json:"username,omitempty"`
	Phone           string                 `json:"phone,omitempty"`
	Status          string                 `json:"status"`
	EmailVerifiedAt *string                `json:"email_verified_at"`
	PhoneVerifiedAt *string                `json:"phone_verified_at"`
	LastLoginAt     *string                `json:"last_login_at"`
	Profile         UserProfileDetail      `json:"profile"`
	Roles           []UserRoleDetail       `json:"roles"`
	Permissions     []UserPermissionDetail `json:"direct_permissions"`
	Sessions        UserSessionSummary     `json:"sessions_summary"`
	Audit           UserAuditSummary       `json:"audit_summary"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
	DeletedAt       *string                `json:"deleted_at"`
}

type UpdateProfileRequest struct {
	Name       *string `json:"name"`
	Phone      *string `json:"phone"`
	Bio        *string `json:"bio"`
	JobTitle   *string `json:"job_title"`
	Department *string `json:"department"`
	Company    *string `json:"company"`
	Address    *string `json:"address"`
	Timezone   *string `json:"timezone"`
	Language   *string `json:"language"`
}

type UpdateAvatarRequest struct {
	AvatarURL string `json:"avatar_url" binding:"required"`
}
