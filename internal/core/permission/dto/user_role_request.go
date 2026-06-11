package dto

type AssignRolesRequest struct {
	RoleIDs []string `json:"role_ids" binding:"required,gt=0"`
}
