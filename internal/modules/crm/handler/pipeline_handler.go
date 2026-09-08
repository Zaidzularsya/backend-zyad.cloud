package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

type PipelineHandler struct {
	svc service.PipelineService
}

func NewPipelineHandler(svc service.PipelineService) *PipelineHandler {
	return &PipelineHandler{svc: svc}
}

// RegisterRoutes registers pipeline routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *PipelineHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/pipelines")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.restore"), h.Restore)
	group.POST("/:id/archive", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.archive"), h.Archive)
	group.PUT("/:id/stages", permissionmiddleware.RequireOrganizationOrGlobal(p, "pipeline.configure_stage"), h.ReplaceStages)
}

func (h *PipelineHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.PipelineListQuery
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

	pipelines, total, err := h.svc.List(c.Request.Context(), scope, repository.PipelineListFilter{
		IncludeArchived: query.IncludeArchived,
		Limit:           perPage,
		Offset:          (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.PipelineListFromDomain(pipelines), dto.BuildMeta(page, perPage, total))
}

func stagesFromRequest(requests []dto.StageRequest) []repository.StageInput {
	stages := make([]repository.StageInput, 0, len(requests))
	for _, r := range requests {
		var id *string
		if r.ID != "" {
			id = &r.ID
		}
		stages = append(stages, repository.StageInput{
			ID:          id,
			Name:        r.Name,
			Position:    r.Position,
			Probability: r.Probability,
			IsWon:       r.IsWon,
			IsLost:      r.IsLost,
		})
	}
	return stages
}

func (h *PipelineHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreatePipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	pipeline, err := h.svc.Create(c.Request.Context(), scope, repository.CreatePipelineParams{
		Name:      req.Name,
		IsDefault: req.IsDefault,
		Stages:    stagesFromRequest(req.Stages),
		CreatedBy: userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.PipelineFromDomain(pipeline))
}

func (h *PipelineHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	pipeline, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.PipelineFromDomain(pipeline))
}

func (h *PipelineHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdatePipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	pipeline, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdatePipelineParams{
		Name:      req.Name,
		IsDefault: req.IsDefault,
		UpdatedBy: userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.PipelineFromDomain(pipeline))
}

func (h *PipelineHandler) Delete(c *gin.Context) {
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

func (h *PipelineHandler) Restore(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Restore(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "restored", nil)
}

func (h *PipelineHandler) Archive(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	pipeline, err := h.svc.Archive(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.PipelineFromDomain(pipeline))
}

func (h *PipelineHandler) ReplaceStages(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.ReplaceStagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	pipeline, err := h.svc.ReplaceStages(c.Request.Context(), scope, c.Param("id"), stagesFromRequest(req.Stages), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.PipelineFromDomain(pipeline))
}
