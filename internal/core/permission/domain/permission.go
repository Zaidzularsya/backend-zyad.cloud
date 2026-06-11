package domain

import "time"

type Permission struct {
	ID          string
	Name        string
	Description string
	ModuleID    string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

type Role struct {
	ID          string
	Name        string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Permissions []Permission
}

type UserPermissionSet struct {
	UserID      string
	RoleNames   []string
	Permissions []Permission
}
