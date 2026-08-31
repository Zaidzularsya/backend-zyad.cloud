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
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminSubmissionHandler struct {
	submissionSvc service.SubmissionService
}

func NewAdminSubmissionHandler(submissionSvc service.SubmissionService) *AdminSubmissionHandler {
	return &AdminSubmissionHandler{
		submissionSvc: submissionSvc,
	}
}

func (h *AdminSubmissionHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing-submissions")
	
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.read"), h.ListSubmissions)
	group.GET("/export", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.export"), h.ExportSubmissions)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.read"), h.GetSubmission)
	group.PATCH("/:id/status", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.update"), h.UpdateStatus)
	group.POST("/:id/notes", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.update"), h.AddNote)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.submission.delete"), h.DeleteSubmission)
}

func (h *AdminSubmissionHandler) ListSubmissions(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var query dto.SubmissionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	limit := query.PerPage
	if limit <= 0 {
		limit = 10
	}
	offset := (query.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	filter := repository.SubmissionFilter{
		LandingPageID: query.LandingPageID,
		FormID:        query.FormID,
		Status:        domain.SubmissionStatus(query.Status),
		Limit:         limit,
		Offset:        offset,
	}

	submissions, err := h.submissionSvc.ListSubmissions(c.Request.Context(), scope, filter)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	// Assuming submissionSvc.ListSubmissions returns list without total for now.
	// We'll wrap in simple OK. Ideally, use Pagination helper if repository returns count.
	corehttp.OK(c, "Submissions retrieved successfully", submissions)
}

func (h *AdminSubmissionHandler) GetSubmission(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	id := c.Param("id")

	submission, err := h.submissionSvc.GetSubmission(c.Request.Context(), scope, id)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Submission retrieved successfully", submission)
}

func (h *AdminSubmissionHandler) UpdateStatus(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	id := c.Param("id")

	var req dto.UpdateSubmissionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	status := domain.SubmissionStatus(req.Status)
	if !status.IsValid() {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid status", http.StatusUnprocessableEntity))
		return
	}

	submission, err := h.submissionSvc.UpdateSubmissionStatus(c.Request.Context(), scope, id, status)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	// If reason is provided, we can add a note as well
	if req.Reason != "" {
		_ = h.submissionSvc.AddSubmissionNote(c.Request.Context(), scope, id, req.Reason, permissionmiddleware.UserID(c))
	}

	corehttp.OK(c, "Submission status updated successfully", submission)
}

func (h *AdminSubmissionHandler) AddNote(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	id := c.Param("id")

	var req dto.CreateSubmissionNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	err = h.submissionSvc.AddSubmissionNote(c.Request.Context(), scope, id, req.Note, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Note added successfully", nil)
}

func (h *AdminSubmissionHandler) DeleteSubmission(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	id := c.Param("id")

	if err := h.submissionSvc.DeleteSubmission(c.Request.Context(), scope, id); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Submission deleted successfully", nil)
}

func (h *AdminSubmissionHandler) ExportSubmissions(c *gin.Context) {
	// MVP: Not implemented yet, returning 501 Not Implemented
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "Export functionality is not yet implemented", http.StatusNotImplemented))
}
