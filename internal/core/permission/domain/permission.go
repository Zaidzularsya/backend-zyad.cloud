package domain

import "time"

type Permission struct {
	ID          string
	Name        string
	Slug        string
	Module      string
	Action      string
	Description string
	ModuleID    string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

type Role struct {
	ID          string
	Name        string
	Slug        string
	Description string
	IsSystem    bool
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Permissions []Permission
}

type UserPermissionSet struct {
	UserID      string
	RoleNames   []string
	Permissions []Permission
}

type UserRole struct {
	ID             string
	UserID         string
	RoleID         string
	RoleSlug       string
	OrganizationID *string
	AssignedBy     *string
	AssignedAt     time.Time
}

type UserPermission struct {
	ID             string
	UserID         string
	PermissionID   string
	PermissionSlug string
	OrganizationID *string
	Effect         string // "allow" or "deny"
	AssignedBy     *string
	AssignedAt     time.Time
	CreatedAt      time.Time
}
