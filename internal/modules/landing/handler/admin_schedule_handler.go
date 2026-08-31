package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminScheduleHandler struct {
	revisionService service.RevisionService
}

func NewAdminScheduleHandler(revisionService service.RevisionService) *AdminScheduleHandler {
	return &AdminScheduleHandler{
		revisionService: revisionService,
	}
}

func (h *AdminScheduleHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing-pages/:id/schedules")

	group.DELETE("/:scheduleId", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.page.publish"), h.CancelSchedule)
}

func (h *AdminScheduleHandler) Schedule(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	pageID := c.Param("id")

	var req dto.ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// For MVP, we will assume only one schedule is created per request
	var lastSchedule domain.LandingPageSchedule
	var processErr error

	if req.PublishAt != nil {
		params := service.SchedulePublishParams{
			PageID:      pageID,
			Action:      domain.ScheduleActionPublish,
			ScheduledAt: *req.PublishAt,
			ActorID:     userID,
		}
		lastSchedule, processErr = h.revisionService.ScheduleAction(c.Request.Context(), scope, params)
	}

	if req.UnpublishAt != nil && processErr == nil {
		params := service.SchedulePublishParams{
			PageID:      pageID,
			Action:      domain.ScheduleActionUnpublish,
			ScheduledAt: *req.UnpublishAt,
			ActorID:     userID,
		}
		lastSchedule, processErr = h.revisionService.ScheduleAction(c.Request.Context(), scope, params)
	}

	if processErr != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", processErr.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", lastSchedule)
}

func (h *AdminScheduleHandler) CancelSchedule(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	scheduleID := c.Param("scheduleId")

	if err := h.revisionService.CancelSchedule(c.Request.Context(), scope, scheduleID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "cancelled", nil)
}
