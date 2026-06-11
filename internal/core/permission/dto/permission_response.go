package dto

import "zyad.cloud/internal/core/permission/domain"

type PermissionResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ModuleID    string `json:"module_id,omitempty"`
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

func NewPermissionResponse(permission domain.Permission) PermissionResponse {
	return PermissionResponse{
		ID:          permission.ID,
		Name:        permission.Name,
		Description: permission.Description,
		ModuleID:    permission.ModuleID,
	}
}

func NewPermissionResponses(permissions []domain.Permission) []PermissionResponse {
	responses := make([]PermissionResponse, 0, len(permissions))
	for _, permission := range permissions {
		responses = append(responses, NewPermissionResponse(permission))
	}
	return responses
}
