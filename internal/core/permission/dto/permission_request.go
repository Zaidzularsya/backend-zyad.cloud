package dto

type CreatePermissionRequest struct {
	Name        string `json:"name" binding:"required,min=3,max=150"`
	Description string `json:"description" binding:"max=500"`
	ModuleID    string `json:"module_id"`
}

type UpdatePermissionRequest struct {
	Description string `json:"description" binding:"max=500"`
	ModuleID    string `json:"module_id"`
}
