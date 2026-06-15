package dto

type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=3,max=150"`
	Slug        string `json:"slug" binding:"required,min=3,max=150"`
	Description string `json:"description" binding:"max=500"`
	IsSystem    bool   `json:"is_system"`
}

type UpdateRoleRequest struct {
	Name        string `json:"name" binding:"omitempty,min=3,max=150"`
	Slug        string `json:"slug" binding:"omitempty,min=3,max=150"`
	Description string `json:"description" binding:"max=500"`
}

type AssignPermissionsRequest struct {
	PermissionIDs []string `json:"permission_ids" binding:"required,gt=0"`
}

type AssignRolePermissionRequest struct {
	PermissionID string `json:"permission_id" binding:"required,uuid"`
	Scope        string `json:"scope" binding:"required,oneof=none own team branch department organization all"`
}
