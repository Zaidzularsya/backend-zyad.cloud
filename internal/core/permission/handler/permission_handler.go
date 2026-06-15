package handler

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/core/permission/dto"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/core/permission/service"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *service.Service
	checker permissionmiddleware.PermissionChecker
}

func New(permissionService *service.Service) *Handler {
	return &Handler{
		service: permissionService,
		checker: permissionService,
	}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	// Roles CRUD
	router.GET("/admin/roles", permissionmiddleware.Require(h.checker, "role.read"), h.ListRoles)
	router.POST("/admin/roles", permissionmiddleware.Require(h.checker, "role.create"), h.CreateRole)
	router.GET("/admin/roles/:id", permissionmiddleware.Require(h.checker, "role.read"), h.GetRoleByID)
	router.PATCH("/admin/roles/:id", permissionmiddleware.Require(h.checker, "role.update"), h.UpdateRole)
	router.DELETE("/admin/roles/:id", permissionmiddleware.Require(h.checker, "role.delete"), h.DeleteRole)

	// Role Permissions
	router.GET("/admin/roles/:id/permissions", permissionmiddleware.Require(h.checker, "role.read"), h.GetRolePermissionsByID)
	router.POST("/admin/roles/:id/permissions", permissionmiddleware.Require(h.checker, "permission.manage"), h.AssignRolePermission)
	router.DELETE("/admin/roles/:id/permissions/:permissionId", permissionmiddleware.Require(h.checker, "permission.manage"), h.RevokeRolePermission)

	// Permissions CRUD & Information
	router.GET("/admin/permissions", permissionmiddleware.Require(h.checker, "permission.read"), h.ListPermissions)
	router.GET("/admin/permissions/:id", permissionmiddleware.Require(h.checker, "permission.read"), h.GetPermissionByID)
	router.POST("/admin/permissions", permissionmiddleware.Require(h.checker, "permission.manage"), h.CreatePermission)
	router.PATCH("/admin/permissions/:id", permissionmiddleware.Require(h.checker, "permission.manage"), h.UpdatePermission)
	router.DELETE("/admin/permissions/:id", permissionmiddleware.Require(h.checker, "permission.manage"), h.DeletePermission)
	router.GET("/admin/permissions/grouped", permissionmiddleware.Require(h.checker, "permission.read"), h.ListGroupedPermissions)
	router.GET("/admin/permission-matrix", permissionmiddleware.Require(h.checker, "permission.read"), h.GetPermissionMatrix)

	// User Roles
	router.GET("/admin/users/:id/roles", permissionmiddleware.Require(h.checker, "role.read"), h.ListUserRoles)
	router.POST("/admin/users/:id/roles", permissionmiddleware.Require(h.checker, "role.assign"), h.AssignUserRole)
	router.DELETE("/admin/users/:id/roles/:roleId", permissionmiddleware.Require(h.checker, "role.assign"), h.RevokeUserRole)

	// User Direct Permissions Override
	router.GET("/admin/users/:id/permissions", permissionmiddleware.Require(h.checker, "permission.read"), h.ListUserPermissions)
	router.POST("/admin/users/:id/permissions", permissionmiddleware.Require(h.checker, "permission.manage"), h.AssignUserPermission)
	router.DELETE("/admin/users/:id/permissions/:permissionId", permissionmiddleware.Require(h.checker, "permission.manage"), h.RevokeUserPermission)
}

// === ROLES ===

func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.service.ListRoles(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "roles retrieved successfully", dto.NewRoleResponses(roles))
}

func (h *Handler) GetRoleByID(c *gin.Context) {
	role, err := h.service.GetRoleByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "role retrieved successfully", dto.NewRoleResponse(role))
}

func (h *Handler) CreateRole(c *gin.Context) {
	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	role := domain.Role{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		IsSystem:    req.IsSystem,
	}

	if err := h.service.CreateRole(c.Request.Context(), &role); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "role created successfully", dto.NewRoleResponse(role))
}

func (h *Handler) UpdateRole(c *gin.Context) {
	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// Fetch existing
	existing, err := h.service.GetRoleByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Slug != "" {
		existing.Slug = req.Slug
	}
	existing.Description = req.Description

	if err := h.service.UpdateRole(c.Request.Context(), c.Param("id"), &existing); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "role updated successfully", dto.NewRoleResponse(existing))
}

func (h *Handler) DeleteRole(c *gin.Context) {
	if err := h.service.DeleteRole(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "role deleted successfully", nil)
}

// === ROLE PERMISSIONS ===

func (h *Handler) GetRolePermissionsByID(c *gin.Context) {
	permissions, err := h.service.GetRolePermissionsByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "role permissions retrieved successfully", dto.NewPermissionResponses(permissions))
}

func (h *Handler) AssignRolePermission(c *gin.Context) {
	var req dto.AssignRolePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.service.AssignRolePermission(c.Request.Context(), c.Param("id"), req.PermissionID, req.Scope); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "permission assigned to role successfully", nil)
}

func (h *Handler) RevokeRolePermission(c *gin.Context) {
	if err := h.service.RevokeRolePermission(c.Request.Context(), c.Param("id"), c.Param("permissionId")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "permission revoked from role successfully", nil)
}

// === PERMISSIONS ===

func (h *Handler) ListPermissions(c *gin.Context) {
	permissions, err := h.service.ListPermissions(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "permissions retrieved successfully", dto.NewPermissionResponses(permissions))
}

func (h *Handler) GetPermissionByID(c *gin.Context) {
	permission, err := h.service.GetPermissionByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "permission retrieved successfully", dto.NewPermissionResponse(permission))
}

func (h *Handler) CreatePermission(c *gin.Context) {
	var req dto.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	permission := domain.Permission{
		Name:        req.Name,
		Description: req.Description,
		ModuleID:    req.ModuleID,
	}

	if err := h.service.CreatePermission(c.Request.Context(), &permission); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "permission created successfully", dto.NewPermissionResponse(permission))
}

func (h *Handler) UpdatePermission(c *gin.Context) {
	var req dto.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	existing, err := h.service.GetPermissionByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	existing.Description = req.Description
	existing.ModuleID = req.ModuleID

	if err := h.service.UpdatePermission(c.Request.Context(), c.Param("id"), &existing); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permission updated successfully", dto.NewPermissionResponse(existing))
}

func (h *Handler) DeletePermission(c *gin.Context) {
	if err := h.service.DeletePermission(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "permission deleted successfully", nil)
}

func (h *Handler) ListGroupedPermissions(c *gin.Context) {
	grouped, err := h.service.ListGroupedPermissions(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response := make(dto.GroupedPermissionsResponse)
	for module, perms := range grouped {
		response[module] = dto.NewPermissionResponses(perms)
	}

	corehttp.OK(c, "grouped permissions retrieved successfully", response)
}

func (h *Handler) GetPermissionMatrix(c *gin.Context) {
	roles, perms, matrix, err := h.service.GetPermissionMatrix(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "permission matrix retrieved successfully", dto.PermissionMatrixResponse{
		Roles:       dto.NewRoleResponses(roles),
		Permissions: dto.NewPermissionResponses(perms),
		Matrix:      matrix,
	})
}

// === USER ROLES ===

func (h *Handler) ListUserRoles(c *gin.Context) {
	roles, err := h.service.ListUserRoles(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "user roles retrieved successfully", dto.NewUserRoleResponses(roles))
}

func (h *Handler) AssignUserRole(c *gin.Context) {
	var req dto.AssignUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	assignedBy := permissionmiddleware.UserID(c)

	if err := h.service.AssignUserRole(c.Request.Context(), c.Param("id"), req.RoleID, req.OrganizationID, assignedBy); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "role assigned to user successfully", nil)
}

func (h *Handler) RevokeUserRole(c *gin.Context) {
	if err := h.service.RevokeUserRole(c.Request.Context(), c.Param("id"), c.Param("roleId")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "role revoked from user successfully", nil)
}

// === USER DIRECT PERMISSIONS ===

func (h *Handler) ListUserPermissions(c *gin.Context) {
	permissions, err := h.service.ListUserPermissions(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "user direct permissions retrieved successfully", dto.NewUserPermissionResponses(permissions))
}

func (h *Handler) AssignUserPermission(c *gin.Context) {
	var req dto.AssignUserPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	assignedBy := permissionmiddleware.UserID(c)

	if err := h.service.AssignUserPermission(c.Request.Context(), c.Param("id"), req.PermissionID, req.Effect, req.OrganizationID, assignedBy); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "direct permission assigned to user successfully", nil)
}

func (h *Handler) RevokeUserPermission(c *gin.Context) {
	if err := h.service.RevokeUserPermission(c.Request.Context(), c.Param("id"), c.Param("permissionId")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "direct permission revoked from user successfully", nil)
}
