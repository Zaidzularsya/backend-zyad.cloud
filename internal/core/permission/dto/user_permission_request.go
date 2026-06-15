package dto

import (
	"time"
	"zyad.cloud/internal/core/permission/domain"
)

type AssignUserPermissionRequest struct {
	PermissionID   string  `json:"permission_id" binding:"required,uuid"`
	Effect         string  `json:"effect" binding:"required,oneof=allow deny"`
	OrganizationID *string `json:"organization_id" binding:"omitempty,uuid"`
}

type UserPermissionResponse struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	PermissionID   string  `json:"permission_id"`
	PermissionSlug string  `json:"permission_slug"`
	OrganizationID *string `json:"organization_id,omitempty"`
	Effect         string  `json:"effect"`
	AssignedBy     *string `json:"assigned_by,omitempty"`
	AssignedAt     string  `json:"assigned_at"`
}

func NewUserPermissionResponse(up domain.UserPermission) UserPermissionResponse {
	return UserPermissionResponse{
		ID:             up.ID,
		UserID:         up.UserID,
		PermissionID:   up.PermissionID,
		PermissionSlug: up.PermissionSlug,
		OrganizationID: up.OrganizationID,
		Effect:         up.Effect,
		AssignedBy:     up.AssignedBy,
		AssignedAt:     up.AssignedAt.UTC().Format(time.RFC3339),
	}
}

func NewUserPermissionResponses(ups []domain.UserPermission) []UserPermissionResponse {
	responses := make([]UserPermissionResponse, 0, len(ups))
	for _, up := range ups {
		responses = append(responses, NewUserPermissionResponse(up))
	}
	return responses
}
