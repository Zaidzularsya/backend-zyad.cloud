package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/businesstime"
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
	svc  service.LeadService
	perm permissionmiddleware.CombinedPermissionChecker
}

func NewLeadHandler(svc service.LeadService) *LeadHandler {
	return &LeadHandler{svc: svc}
}

// RegisterRoutes registers lead routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *LeadHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	group := router.Group("/leads")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.restore"), h.Restore)
	group.POST("/:id/assign", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.assign"), h.Assign)
	group.POST("/:id/convert", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.convert"), h.Convert)
	group.POST("/:id/deal", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.create"), h.CreateDeal)
	group.POST("/:id/disqualify", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.Disqualify)
	group.POST("/:id/playbook/start", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.StartPlaybook)
	group.GET("/:id/events", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.ListEvents)
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

	if perPage > maxLeadPerPage {
		perPage = maxLeadPerPage
	}
	if !repository.IsValidLeadSort(query.Sort) {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid sort", http.StatusUnprocessableEntity))
		return
	}
	createdFrom, createdToExcl, err := parseLeadCreatedRange(query.CreatedFrom, query.CreatedTo)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	leads, total, err := h.svc.List(c.Request.Context(), scope, repository.LeadListFilter{
		Search:             query.Search,
		Status:             domain.LeadStatus(query.Status),
		OwnerUserID:        query.OwnerUserID,
		Source:             query.Source,
		ConvertedContactID: query.ConvertedContactID,
		CreatedFrom:        createdFrom,
		CreatedToExclusive: createdToExcl,
		Sort:               query.Sort,
		Limit:              perPage,
		Offset:             (page - 1) * perPage,
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
		ContactName:   req.ContactName,
		CompanyName:   req.CompanyName,
		Email:         req.Email,
		Phone:         req.Phone,
		Source:        req.Source,
		Score:         req.Score,
		OwnerUserID:   req.OwnerUserID,
		Notes:         req.Notes,
		JobTitle:      req.JobTitle,
		AnnualRevenue: req.AnnualRevenue,
		Address:       req.Address,
		CreatedBy:     userID,
	})
	if err != nil {
		failLeadError(c, err)
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
		ContactName:   req.ContactName,
		CompanyName:   req.CompanyName,
		Email:         req.Email,
		Phone:         req.Phone,
		Source:        req.Source,
		Status:        status,
		Score:         req.Score,
		OwnerUserID:   req.OwnerUserID,
		Notes:         req.Notes,
		JobTitle:      req.JobTitle,
		AnnualRevenue: req.AnnualRevenue,
		Address:       req.Address,
		UpdatedBy:     userID,

		RequirementSummary: req.RequirementSummary,
		BudgetEstimate:     req.BudgetEstimate,
		TargetDate:         req.TargetDate,
		DecisionMaker:      req.DecisionMaker,
	})
	if err != nil {
		failLeadError(c, err)
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
		failLeadError(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

// allowed memeriksa permission tambahan yang bergantung pada isi body
// (urutan sama dengan middleware: organization dulu, lalu global).
func (h *LeadHandler) allowed(c *gin.Context, perms ...string) bool {
	if h.perm == nil || len(perms) == 0 {
		return true
	}
	ctx := c.Request.Context()
	userID := permissionmiddleware.UserID(c)
	if tc, ok := coretenant.FromContext(ctx); ok {
		if h.perm.CanOrganization(ctx, userID, tc.OrganizationID(), perms) == nil {
			return true
		}
	}
	return h.perm.Can(ctx, userID, perms) == nil
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

	var extra []string
	if req.Deal != nil {
		extra = append(extra, "deal.create")
	}
	if (req.Company != nil && req.Company.Mode == "new") || (req.Company == nil && req.CreateCompany) {
		extra = append(extra, "company.create")
	}
	if !h.allowed(c, extra...) {
		corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden))
		return
	}

	ownerUserID := req.OwnerUserID
	if ownerUserID == "" {
		ownerUserID = userID
	}
	params := service.ConvertLeadParams{CreateCompany: req.CreateCompany, OwnerUserID: ownerUserID, ConvertedBy: userID}
	if req.Company != nil {
		params.Company = &service.ConvertCompanyInput{
			Mode: req.Company.Mode, CompanyID: req.Company.CompanyID, Name: req.Company.Name,
			Industry: req.Company.Industry, Website: req.Company.Website, Phone: req.Company.Phone,
		}
	}
	if req.Deal != nil {
		deal := convertDealInput(*req.Deal)
		if deal.OwnerUserID == "" {
			deal.OwnerUserID = ownerUserID
		}
		params.Deal = &deal
	}

	result, err := h.svc.Convert(c.Request.Context(), scope, c.Param("id"), params)
	if err != nil {
		failLeadError(c, err)
		return
	}
	corehttp.OK(c, "success", dto.LeadConversionFromDomain(result))
}

func convertDealInput(r dto.ConvertDealRequest) service.ConvertDealInput {
	return service.ConvertDealInput{
		PipelineID: r.PipelineID, StageID: r.StageID, Title: r.Title, Value: r.Value,
		ExpectedCloseDate: r.ExpectedCloseDate, Description: r.Description,
		DecisionMaker: r.DecisionMaker, OwnerUserID: r.OwnerUserID,
	}
}

func (h *LeadHandler) CreateDeal(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req dto.ConvertDealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	userID := permissionmiddleware.UserID(c)
	input := convertDealInput(req)
	if input.OwnerUserID == "" {
		input.OwnerUserID = userID
	}
	lead, deal, err := h.svc.CreateDealForLead(c.Request.Context(), scope, c.Param("id"), input, userID)
	if err != nil {
		failLeadError(c, err)
		return
	}
	corehttp.OK(c, "success", dto.LeadDealResponse{Lead: dto.LeadFromDomain(lead), Deal: dto.DealFromDomain(deal)})
}

// failLeadError memetakan error validasi service lead ke 422; error lain
// diteruskan apa adanya (AppError dari MapNotFound, dsb).
func failLeadError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrLeadOwnerNotMember) || errors.Is(err, service.ErrInvalidAnnualRevenue) ||
		errors.Is(err, service.ErrInvalidBudgetEstimate) || errors.Is(err, service.ErrInvalidTargetDate) ||
		errors.Is(err, service.ErrInvalidDealInput) || errors.Is(err, service.ErrInvalidCompanyInput) {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	corehttp.Fail(c, mapLeadError(err))
}

const maxLeadPerPage = 100

// parseLeadCreatedRange parses optional YYYY-MM-DD bounds. The upper bound
// is returned as the start of the following day so the range is inclusive.
// Days are Asia/Jakarta days, returned as UTC instants to compare with
// created_at (stored as UTC).
func parseLeadCreatedRange(from, to string) (time.Time, time.Time, error) {
	var fromDate, toExcl time.Time
	if from != "" {
		d, err := time.Parse("2006-01-02", from)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("created_from must be a date in YYYY-MM-DD format")
		}
		fromDate = businesstime.DayStartUTC(d)
	}
	if to != "" {
		d, err := time.Parse("2006-01-02", to)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("created_to must be a date in YYYY-MM-DD format")
		}
		toExcl = businesstime.DayStartUTC(d.AddDate(0, 0, 1))
	}
	if !fromDate.IsZero() && !toExcl.IsZero() && !fromDate.Before(toExcl) {
		return time.Time{}, time.Time{}, errors.New("created_from must not be after created_to")
	}
	return fromDate, toExcl, nil
}

func (h *LeadHandler) Disqualify(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req dto.DisqualifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	lead, err := h.svc.Disqualify(c.Request.Context(), scope, c.Param("id"), repository.DisqualifyLeadParams{
		Reason: domain.DisqualifyReason(req.Reason), Note: req.Note, UpdatedBy: permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, mapLeadError(err))
		return
	}
	corehttp.OK(c, "success", dto.LeadFromDomain(lead))
}

func (h *LeadHandler) StartPlaybook(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	run, err := h.svc.StartPlaybook(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, mapLeadError(err))
		return
	}
	corehttp.OK(c, "success", dto.PlaybookRunFromDomain(&run))
}

func (h *LeadHandler) ListEvents(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	events, total, err := h.svc.ListEvents(c.Request.Context(), scope, c.Param("id"), page, perPage)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	out := make([]dto.LeadEventResponse, 0, len(events))
	for _, e := range events {
		out = append(out, dto.LeadEventFromDomain(e))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func mapLeadError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidDisqualifyReason):
		return coreerrors.New("DISQUALIFY_REASON_INVALID", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrUseDisqualifyEndpoint):
		return coreerrors.New("USE_DISQUALIFY_ENDPOINT", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrLeadAlreadyConverted):
		return coreerrors.New("LEAD_ALREADY_CONVERTED", err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrInvalidPipelineStage):
		return coreerrors.New("INVALID_PIPELINE_STAGE", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrInvalidStartStage):
		return coreerrors.New("INVALID_START_STAGE", err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrLeadNotConverted):
		return coreerrors.New("LEAD_NOT_CONVERTED", err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrLeadDealExists):
		return coreerrors.New("LEAD_DEAL_EXISTS", err.Error(), http.StatusConflict)
	}
	return mapPlaybookError(err)
}
