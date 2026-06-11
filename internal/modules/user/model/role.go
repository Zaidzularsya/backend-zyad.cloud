package model

import "time"

type Role struct {
	ID          string
	Name        string
	Slug        string
	Description string
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Permissions []Permission
}

type UserRole struct {
	ID             string
	UserID         string
	RoleID         string
	RoleSlug       string
	OrganizationID string
	AssignedBy     string
	AssignedAt     time.Time
}
