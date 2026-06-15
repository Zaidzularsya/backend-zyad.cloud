package dto

import (
	"time"
	"zyad.cloud/internal/core/permission/domain"
)

type PermissionResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Module      string `json:"module"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type RolePermissionsResponse struct {
	RoleID      string               `json:"role_id"`
	RoleName    string               `json:"role_name"`
	Permissions []PermissionResponse `json:"permissions"`
}

type UserPermissionsResponse struct {
	UserID      string               `json:"user_id"`
	RoleNames   []string             `json:"role_names"`
	Permissions []PermissionResponse `json:"permissions"`
}

type GroupedPermissionsResponse map[string][]PermissionResponse

type PermissionMatrixResponse struct {
	Roles       []RoleResponse               `json:"roles"`
	Permissions []PermissionResponse         `json:"permissions"`
	Matrix      map[string]map[string]string `json:"matrix"` // role_id -> permission_id -> scope
}

func NewPermissionResponse(permission domain.Permission) PermissionResponse {
	createdAtStr := ""
	if permission.CreatedAt != nil {
		createdAtStr = permission.CreatedAt.UTC().Format(time.RFC3339)
	}
	updatedAtStr := ""
	if permission.UpdatedAt != nil {
		updatedAtStr = permission.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return PermissionResponse{
		ID:          permission.ID,
		Name:        permission.Name,
		Slug:        permission.Slug,
		Module:      permission.Module,
		Action:      permission.Action,
		Description: permission.Description,
		CreatedAt:   createdAtStr,
		UpdatedAt:   updatedAtStr,
	}
}

func NewPermissionResponses(permissions []domain.Permission) []PermissionResponse {
	responses := make([]PermissionResponse, 0, len(permissions))
	for _, permission := range permissions {
		responses = append(responses, NewPermissionResponse(permission))
	}
	return responses
}
