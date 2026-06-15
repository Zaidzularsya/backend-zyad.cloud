package dto

import (
	"time"
	"zyad.cloud/internal/core/permission/domain"
)

type RoleResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Slug        string               `json:"slug"`
	Description string               `json:"description,omitempty"`
	IsSystem    bool                 `json:"is_system"`
	CreatedAt   string               `json:"created_at,omitempty"`
	UpdatedAt   string               `json:"updated_at,omitempty"`
	Permissions []PermissionResponse `json:"permissions,omitempty"`
}

func NewRoleResponse(role domain.Role) RoleResponse {
	createdAtStr := ""
	if role.CreatedAt != nil {
		createdAtStr = role.CreatedAt.UTC().Format(time.RFC3339)
	}
	updatedAtStr := ""
	if role.UpdatedAt != nil {
		updatedAtStr = role.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Slug:        role.Slug,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		CreatedAt:   createdAtStr,
		UpdatedAt:   updatedAtStr,
		Permissions: NewPermissionResponses(role.Permissions),
	}
}

func NewRoleResponses(roles []domain.Role) []RoleResponse {
	responses := make([]RoleResponse, 0, len(roles))
	for _, role := range roles {
		responses = append(responses, NewRoleResponse(role))
	}
	return responses
}
