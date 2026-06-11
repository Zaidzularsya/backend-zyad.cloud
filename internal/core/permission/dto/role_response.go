package dto

import "zyad.cloud/internal/core/permission/domain"

type RoleResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Permissions []PermissionResponse `json:"permissions,omitempty"`
}

func NewRoleResponse(role domain.Role) RoleResponse {
	return RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
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
