package model

import "time"

type PermissionEffect string

const (
	PermissionEffectAllow PermissionEffect = "allow"
	PermissionEffectDeny  PermissionEffect = "deny"
)

type Permission struct {
	ID          string
	Module      string
	Action      string
	Name        string
	Slug        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserPermission struct {
	ID             string
	UserID         string
	PermissionID   string
	PermissionSlug string
	OrganizationID string
	Effect         PermissionEffect
	AssignedBy     string
	AssignedAt     time.Time
	CreatedAt      time.Time
}

func (e PermissionEffect) IsValid() bool {
	switch e {
	case PermissionEffectAllow, PermissionEffectDeny:
		return true
	default:
		return false
	}
}
