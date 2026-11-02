package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/businesstime"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// PreviewService is the subset of service.PreviewService used by the handler.
type PreviewService interface {
	Summary(ctx context.Context, scope coretenant.Scope) (service.CandidateSummary, error)
	List(ctx context.Context, scope coretenant.Scope, f service.CandidateFilter, page, perPage int) ([]service.CandidateRow, int64, error)
	ExportCSV(ctx context.Context, scope coretenant.Scope, f service.CandidateFilter, w io.Writer, begin func(total int64)) error
}

// PreviewHandler serves the isolation candidate preview (read-only).
type PreviewHandler struct {
	svc PreviewService
	now func() time.Time
}

func NewPreviewHandler(svc PreviewService) *PreviewHandler {
	return &PreviewHandler{svc: svc, now: time.Now}
}

// RegisterRoutes mounts the preview under /control. Nothing here contacts a
// router, so probeLimit is not applied.
func (h *PreviewHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	read := permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_control.read")
	group := router.Group("/control/preview")
	group.GET("", read, h.List)
	group.GET("/summary", read, h.Summary)
	group.GET("/export", read, h.Export)
}

func (h *PreviewHandler) Summary(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	s, err := h.svc.Summary(c.Request.Context(), scope)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PreviewSummaryFromService(s))
}

func bindPreviewQuery(c *gin.Context) (coretenant.Scope, dto.PreviewQuery, bool) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return coretenant.Scope{}, dto.PreviewQuery{}, false
	}
	var q dto.PreviewQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, err)
		return coretenant.Scope{}, dto.PreviewQuery{}, false
	}
	return scope, q, true
}

func (h *PreviewHandler) List(c *gin.Context) {
	scope, q, ok := bindPreviewQuery(c)
	if !ok {
		return
	}
	// Same clamp as the service, so meta.per_page is the real page size.
	page, perPage := service.ClampCandidatePage(q.Page, q.PerPage)
	rows, total, err := h.svc.List(c.Request.Context(), scope, q.Filter(), page, perPage)
	if err != nil {
		failRouter(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.PreviewListFromService(rows), dto.BuildMeta(page, perPage, total))
}

// Export streams the filtered candidates as CSV. Errors before the first
// byte are answered as JSON; a failure mid-stream can only be logged (the
// status line is already sent), leaving a truncated file.
func (h *PreviewHandler) Export(c *gin.Context) {
	scope, q, ok := bindPreviewQuery(c)
	if !ok {
		return
	}
	filename := fmt.Sprintf("isolir-kandidat-%s.csv", businesstime.DayOf(h.now()).Format("20060102"))
	started := false
	err := h.svc.ExportCSV(c.Request.Context(), scope, q.Filter(), c.Writer, func(total int64) {
		started = true
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
		c.Header("X-Total-Count", strconv.FormatInt(total, 10))
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusOK)
		c.Writer.WriteHeaderNow()
	})
	if err == nil {
		return
	}
	if !started {
		failRouter(c, err)
		return
	}
	slog.ErrorContext(c.Request.Context(), "mikrotik: candidate export aborted mid-stream", "error", err.Error())
}
