package handler

import (
	"errors"
	"net/http"

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

type LeadHandler struct {
	svc service.LeadService
}

func NewLeadHandler(svc service.LeadService) *LeadHandler {
	return &LeadHandler{svc: svc}
}

// RegisterRoutes registers lead routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *LeadHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/leads")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.restore"), h.Restore)
	group.POST("/:id/assign", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.assign"), h.Assign)
	group.POST("/:id/convert", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.convert"), h.Convert)
}

func (h *LeadHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.LeadListQuery
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

	leads, total, err := h.svc.List(c.Request.Context(), scope, repository.LeadListFilter{
		Search:      query.Search,
		Status:      domain.LeadStatus(query.Status),
		OwnerUserID: query.OwnerUserID,
		Limit:       perPage,
		Offset:      (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.LeadListFromDomain(leads), dto.BuildMeta(page, perPage, total))
}

func (h *LeadHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	lead, err := h.svc.Create(c.Request.Context(), scope, repository.CreateLeadParams{
		ContactName: req.ContactName,
		CompanyName: req.CompanyName,
		Email:       req.Email,
		Phone:       req.Phone,
		Source:      req.Source,
		Score:       req.Score,
		OwnerUserID: req.OwnerUserID,
		Notes:       req.Notes,
		CreatedBy:   userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

func (h *LeadHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	lead, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

func (h *LeadHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var status *domain.LeadStatus
	if req.Status != nil {
		s := domain.LeadStatus(*req.Status)
		if !s.IsValid() {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid status", http.StatusUnprocessableEntity))
			return
		}
		status = &s
	}

	lead, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateLeadParams{
		ContactName: req.ContactName,
		CompanyName: req.CompanyName,
		Email:       req.Email,
		Phone:       req.Phone,
		Source:      req.Source,
		Status:      status,
		Score:       req.Score,
		OwnerUserID: req.OwnerUserID,
		Notes:       req.Notes,
		UpdatedBy:   userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

func (h *LeadHandler) Delete(c *gin.Context) {
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

func (h *LeadHandler) Restore(c *gin.Context) {
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

func (h *LeadHandler) Assign(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.AssignLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	lead, err := h.svc.Assign(c.Request.Context(), scope, c.Param("id"), req.OwnerUserID, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

func (h *LeadHandler) Convert(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.ConvertLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	ownerUserID := req.OwnerUserID
	if ownerUserID == "" {
		ownerUserID = userID
	}

	result, err := h.svc.Convert(c.Request.Context(), scope, c.Param("id"), service.ConvertLeadParams{
		CreateCompany: req.CreateCompany,
		OwnerUserID:   ownerUserID,
		ConvertedBy:   userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrLeadAlreadyConverted) {
			corehttp.Fail(c, coreerrors.New("LEAD_ALREADY_CONVERTED", "lead already converted", http.StatusConflict))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadConversionFromDomain(result))
}
