package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type UserListService interface {
	ListUsers(ctx context.Context, query dto.UserListQuery) ([]dto.UserListItem, dto.PaginationMeta, error)
	GetUser(ctx context.Context, userID string, includeDeleted bool) (dto.UserDetailResponse, error)
	CreateUser(ctx context.Context, req dto.CreateUserRequest, metadata service.CreateUserMetadata) (dto.UserDetailResponse, error)
	UpdateUser(ctx context.Context, userID string, req dto.UpdateUserRequest, metadata service.UpdateUserMetadata) (dto.UserDetailResponse, error)
	DeleteUser(ctx context.Context, userID string, metadata service.UserLifecycleMetadata) error
	RestoreUser(ctx context.Context, userID string, metadata service.UserLifecycleMetadata) (dto.UserDetailResponse, error)
	BulkAction(ctx context.Context, req dto.BulkUserActionRequest, metadata service.UserLifecycleMetadata) (dto.BulkUserActionResponse, error)
	ChangeUserStatus(ctx context.Context, userID string, req dto.UpdateUserStatusRequest, metadata service.UserLifecycleMetadata) (dto.UserDetailResponse, error)
}

type UserHandler struct {
	service UserListService
	checker permissionmiddleware.PermissionChecker
}

func NewUserHandler(userService UserListService, checker permissionmiddleware.PermissionChecker) *UserHandler {
	return &UserHandler{service: userService, checker: checker}
}

func (h *UserHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/admin/users")
	group.GET("", permissionmiddleware.Require(h.checker, "user.read"), h.ListUsers)
	group.POST("", permissionmiddleware.Require(h.checker, "user.create"), h.CreateUser)
	group.POST("/bulk-action", h.BulkAction)
	group.GET("/:id", permissionmiddleware.Require(h.checker, "user.read"), h.GetUser)
	group.PATCH("/:id", permissionmiddleware.Require(h.checker, "user.update"), h.UpdateUser)
	group.DELETE("/:id", permissionmiddleware.Require(h.checker, "user.delete"), h.DeleteUser)
	group.POST("/:id/restore", permissionmiddleware.Require(h.checker, "user.restore"), h.RestoreUser)
	group.PATCH("/:id/status", permissionmiddleware.Require(h.checker, "user.update_status"), h.UpdateUserStatus)
	group.POST("/:id/activate", permissionmiddleware.Require(h.checker, "user.update_status"), h.ActivateUser)
	group.POST("/:id/suspend", permissionmiddleware.Require(h.checker, "user.update_status"), h.SuspendUser)
	group.POST("/:id/ban", permissionmiddleware.Require(h.checker, "user.update_status"), h.BanUser)
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	var query dto.UserListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	if query.IncludeDeleted {
		if err := h.requireRestorePermission(c); err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	users, meta, err := h.service.ListUsers(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "users retrieved successfully", users, meta)
}

func (h *UserHandler) GetUser(c *gin.Context) {
	var query dto.UserDetailQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	if query.IncludeDeleted {
		if err := h.requireRestorePermission(c); err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	user, err := h.service.GetUser(c.Request.Context(), c.Param("id"), query.IncludeDeleted)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "user retrieved successfully", user)
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	user, err := h.service.CreateUser(c.Request.Context(), req, service.CreateUserMetadata{
		ActorUserID: permissionmiddleware.UserID(c),
		IPAddress:   c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, "user created successfully", user, nil)
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	user, err := h.service.UpdateUser(c.Request.Context(), c.Param("id"), req, service.UpdateUserMetadata{
		ActorUserID: permissionmiddleware.UserID(c),
		IPAddress:   c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "user updated successfully", user)
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	if err := h.service.DeleteUser(c.Request.Context(), c.Param("id"), lifecycleMetadata(c)); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "user deleted successfully", nil)
}

func (h *UserHandler) RestoreUser(c *gin.Context) {
	user, err := h.service.RestoreUser(c.Request.Context(), c.Param("id"), lifecycleMetadata(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "user restored successfully", user)
}

func (h *UserHandler) BulkAction(c *gin.Context) {
	var req dto.BulkUserActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	permission := map[string]string{
		"delete":        "user.delete",
		"restore":       "user.restore",
		"update_status": "user.update_status",
	}[req.Action]
	if permission == "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "action is invalid", http.StatusUnprocessableEntity))
		return
	}
	if h.checker == nil {
		corehttp.Fail(c, coreerrors.New("PERMISSION_CHECKER_REQUIRED", "permission checker is required", http.StatusInternalServerError))
		return
	}
	if err := h.checker.Can(c.Request.Context(), permissionmiddleware.UserID(c), []string{permission}); err != nil {
		corehttp.Fail(c, err)
		return
	}

	result, err := h.service.BulkAction(c.Request.Context(), req, lifecycleMetadata(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "bulk user action completed", result)
}

func (h *UserHandler) UpdateUserStatus(c *gin.Context) {
	var req dto.UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	h.changeUserStatus(c, req)
}

func (h *UserHandler) ActivateUser(c *gin.Context) {
	h.changeUserStatus(c, dto.UpdateUserStatusRequest{Status: "active"})
}

func (h *UserHandler) SuspendUser(c *gin.Context) {
	var req dto.UserStatusReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	h.changeUserStatus(c, dto.UpdateUserStatusRequest{Status: "suspended", Reason: req.Reason})
}

func (h *UserHandler) BanUser(c *gin.Context) {
	var req dto.UserStatusReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	h.changeUserStatus(c, dto.UpdateUserStatusRequest{Status: "banned", Reason: req.Reason})
}

func (h *UserHandler) changeUserStatus(c *gin.Context, req dto.UpdateUserStatusRequest) {
	user, err := h.service.ChangeUserStatus(c.Request.Context(), c.Param("id"), req, lifecycleMetadata(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "user status updated successfully", user)
}

func lifecycleMetadata(c *gin.Context) service.UserLifecycleMetadata {
	return service.UserLifecycleMetadata{
		ActorUserID: permissionmiddleware.UserID(c),
		IPAddress:   c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}
}

func (h *UserHandler) requireRestorePermission(c *gin.Context) error {
	if h.checker == nil {
		return coreerrors.New("PERMISSION_CHECKER_REQUIRED", "permission checker is required", http.StatusInternalServerError)
	}
	return h.checker.Can(c.Request.Context(), permissionmiddleware.UserID(c), []string{"user.restore"})
}
