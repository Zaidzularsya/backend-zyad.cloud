package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

type ActivityHandler struct {
	svc service.ActivityService
}

func NewActivityHandler(svc service.ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

// RegisterRoutes registers activity routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *ActivityHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/activities")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.delete"), h.Delete)
	group.POST("/:id/complete", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.complete"), h.Complete)
	group.POST("/:id/cancel", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.cancel"), h.Cancel)
	group.POST("/:id/assign", permissionmiddleware.RequireOrganizationOrGlobal(p, "activity.assign"), h.Assign)
}

func parseActivityTimestamp(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *ActivityHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.ActivityListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	activities, total, err := h.svc.List(c.Request.Context(), scope, repository.ActivityListFilter{
		RelatedEntityType: domain.ActivityEntityType(query.RelatedEntityType),
		RelatedEntityID:   query.RelatedEntityID,
		AssigneeUserID:    query.AssigneeUserID,
		Status:            domain.ActivityStatus(query.Status),
		Limit:             perPage,
		Offset:            (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.ActivityListFromDomain(activities), dto.BuildMeta(page, perPage, total))
}

func (h *ActivityHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	dueAt, err := parseActivityTimestamp(req.DueAt)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid due_at, expected RFC3339", http.StatusUnprocessableEntity))
		return
	}

	activity, err := h.svc.Create(c.Request.Context(), scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityType(req.RelatedEntityType),
		RelatedEntityID:   req.RelatedEntityID,
		Type:              domain.ActivityType(req.Type),
		Subject:           req.Subject,
		Description:       req.Description,
		DueAt:             dueAt,
		AssigneeUserID:    req.AssigneeUserID,
		CreatedBy:         userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidActivityEntityType) || errors.Is(err, service.ErrInvalidActivityType) {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}

func (h *ActivityHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	activity, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}

func (h *ActivityHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var activityType *domain.ActivityType
	if req.Type != nil {
		t := domain.ActivityType(*req.Type)
		activityType = &t
	}

	var dueAt *time.Time
	if req.DueAt != nil {
		parsed, err := parseActivityTimestamp(req.DueAt)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid due_at, expected RFC3339", http.StatusUnprocessableEntity))
			return
		}
		dueAt = parsed
	}

	activity, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateActivityParams{
		Type:           activityType,
		Subject:        req.Subject,
		Description:    req.Description,
		DueAt:          dueAt,
		AssigneeUserID: req.AssigneeUserID,
		UpdatedBy:      userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidActivityType) {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}

func (h *ActivityHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *ActivityHandler) Complete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	activity, err := h.svc.Complete(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}

func (h *ActivityHandler) Cancel(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	activity, err := h.svc.Cancel(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}

func (h *ActivityHandler) Assign(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.AssignActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	activity, err := h.svc.Assign(c.Request.Context(), scope, c.Param("id"), req.AssigneeUserID, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ActivityFromDomain(activity))
}
