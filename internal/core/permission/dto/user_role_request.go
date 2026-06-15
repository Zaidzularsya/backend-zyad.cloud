package dto

import (
	"time"
	"zyad.cloud/internal/core/permission/domain"
)

type AssignUserRoleRequest struct {
	RoleID         string  `json:"role_id" binding:"required,uuid"`
	OrganizationID *string `json:"organization_id" binding:"omitempty,uuid"`
}

type UserRoleResponse struct {
	ID             string  `json:"id"`
	UserID         string  `json:"user_id"`
	RoleID         string  `json:"role_id"`
	RoleSlug       string  `json:"role_slug"`
	OrganizationID *string `json:"organization_id,omitempty"`
	AssignedBy     *string `json:"assigned_by,omitempty"`
	AssignedAt     string  `json:"assigned_at"`
}

func NewUserRoleResponse(ur domain.UserRole) UserRoleResponse {
	return UserRoleResponse{
		ID:             ur.ID,
		UserID:         ur.UserID,
		RoleID:         ur.RoleID,
		RoleSlug:       ur.RoleSlug,
		OrganizationID: ur.OrganizationID,
		AssignedBy:     ur.AssignedBy,
		AssignedAt:     ur.AssignedAt.UTC().Format(time.RFC3339),
	}
}

func NewUserRoleResponses(urs []domain.UserRole) []UserRoleResponse {
	responses := make([]UserRoleResponse, 0, len(urs))
	for _, ur := range urs {
		responses = append(responses, NewUserRoleResponse(ur))
	}
	return responses
}
