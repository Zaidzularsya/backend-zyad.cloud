package dto

type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required,min=3,max=150"`
	Description string `json:"description" binding:"max=500"`
}

type UpdateRoleRequest struct {
	Description string `json:"description" binding:"max=500"`
}

type AssignPermissionsRequest struct {
	PermissionIDs []string `json:"permission_ids" binding:"required,gt=0"`
}
