package handler

import (
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/core/permission/dto"
	"zyad.cloud/internal/core/permission/service"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *service.Service
}

func New(permissionService *service.Service) *Handler {
	return &Handler{service: permissionService}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	// Query Endpoints
	router.GET("/permissions", h.ListPermissions)
	router.GET("/permissions/:id", h.GetPermissionByID)
	router.GET("/permissions/roles/:role_name", h.GetRolePermissions)
	router.GET("/permissions/users/:user_id", h.GetUserPermissions)
	router.GET("/roles", h.ListRoles)
	router.GET("/roles/:id", h.GetRoleByID)

	// Admin CRUD Endpoints
	router.POST("/permissions", h.CreatePermission)
	router.PUT("/permissions/:id", h.UpdatePermission)
	router.DELETE("/permissions/:id", h.DeletePermission)

	router.POST("/roles", h.CreateRole)
	router.PUT("/roles/:id", h.UpdateRole)
	router.DELETE("/roles/:id", h.DeleteRole)

	// Mappings Endpoints
	router.POST("/roles/:id/permissions", h.AssignPermissions)
	router.DELETE("/roles/:id/permissions", h.RevokePermissions)
	router.POST("/users/:user_id/roles", h.AssignUserRoles)
	router.DELETE("/users/:user_id/roles", h.RevokeUserRoles)
}

// === PERMISSION HANDLERS ===

func (h *Handler) ListPermissions(c *gin.Context) {
	permissions, err := h.service.ListPermissions(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permissions retrieved", dto.NewPermissionResponses(permissions))
}

func (h *Handler) GetPermissionByID(c *gin.Context) {
	permission, err := h.service.GetPermissionByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permission retrieved", dto.NewPermissionResponse(permission))
}

func (h *Handler) CreatePermission(c *gin.Context) {
	var req dto.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	p := domain.Permission{
		Name:        req.Name,
		Description: req.Description,
		ModuleID:    req.ModuleID,
	}

	if err := h.service.CreatePermission(c.Request.Context(), &p); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "permission created", dto.NewPermissionResponse(p))
}

func (h *Handler) UpdatePermission(c *gin.Context) {
	var req dto.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	p := domain.Permission{
		Description: req.Description,
		ModuleID:    req.ModuleID,
	}

	if err := h.service.UpdatePermission(c.Request.Context(), c.Param("id"), &p); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permission updated", dto.NewPermissionResponse(p))
}

func (h *Handler) DeletePermission(c *gin.Context) {
	if err := h.service.DeletePermission(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permission deleted", nil)
}

// === ROLE HANDLERS ===

func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.service.ListRoles(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "roles retrieved", dto.NewRoleResponses(roles))
}

func (h *Handler) GetRoleByID(c *gin.Context) {
	role, err := h.service.GetRoleByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "role retrieved", dto.NewRoleResponse(role))
}

func (h *Handler) GetRolePermissions(c *gin.Context) {
	role, err := h.service.GetRolePermissions(c.Request.Context(), c.Param("role_name"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "role permissions retrieved", dto.RolePermissionsResponse{
		RoleID:      role.ID,
		RoleName:    role.Name,
		Permissions: dto.NewPermissionResponses(role.Permissions),
	})
}

func (h *Handler) GetUserPermissions(c *gin.Context) {
	result, err := h.service.GetUserPermissions(c.Request.Context(), c.Param("user_id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "user permissions retrieved", dto.UserPermissionsResponse{
		UserID:      result.UserID,
		RoleNames:   result.RoleNames,
		Permissions: dto.NewPermissionResponses(result.Permissions),
	})
}

func (h *Handler) CreateRole(c *gin.Context) {
	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	role := domain.Role{
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.service.CreateRole(c.Request.Context(), &role); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "role created", dto.NewRoleResponse(role))
}

func (h *Handler) UpdateRole(c *gin.Context) {
	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	role := domain.Role{
		Description: req.Description,
	}

	if err := h.service.UpdateRole(c.Request.Context(), c.Param("id"), &role); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "role updated", dto.NewRoleResponse(role))
}

func (h *Handler) DeleteRole(c *gin.Context) {
	if err := h.service.DeleteRole(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "role deleted", nil)
}

// === MAPPINGS HANDLERS ===

func (h *Handler) AssignPermissions(c *gin.Context) {
	var req dto.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	if err := h.service.AssignPermissions(c.Request.Context(), c.Param("id"), req.PermissionIDs); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permissions assigned to role", nil)
}

func (h *Handler) RevokePermissions(c *gin.Context) {
	var req dto.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	if err := h.service.RevokePermissions(c.Request.Context(), c.Param("id"), req.PermissionIDs); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permissions revoked from role", nil)
}

func (h *Handler) AssignUserRoles(c *gin.Context) {
	var req dto.AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	if err := h.service.AssignUserRoles(c.Request.Context(), c.Param("user_id"), req.RoleIDs); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "roles assigned to user", nil)
}

func (h *Handler) RevokeUserRoles(c *gin.Context) {
	var req dto.AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("INVALID_INPUT", err.Error(), 400))
		return
	}

	if err := h.service.RevokeUserRoles(c.Request.Context(), c.Param("user_id"), req.RoleIDs); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "roles revoked from user", nil)
}

