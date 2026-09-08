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

type DealHandler struct {
	svc service.DealService
}

func NewDealHandler(svc service.DealService) *DealHandler {
	return &DealHandler{svc: svc}
}

// RegisterRoutes registers deal routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *DealHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/deals")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.update"), h.Restore)
	group.POST("/:id/move-stage", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.move_stage"), h.MoveStage)
	group.POST("/:id/close-won", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.close_won"), h.CloseWon)
	group.POST("/:id/close-lost", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.close_lost"), h.CloseLost)
	group.POST("/:id/approve-discount", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.approve_discount"), h.ApproveDiscount)
}

func parseDealDate(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *DealHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.DealListQuery
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

	deals, total, err := h.svc.List(c.Request.Context(), scope, repository.DealListFilter{
		Search:      query.Search,
		PipelineID:  query.PipelineID,
		StageID:     query.StageID,
		Status:      domain.DealStatus(query.Status),
		OwnerUserID: query.OwnerUserID,
		Limit:       perPage,
		Offset:      (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.DealListFromDomain(deals), dto.BuildMeta(page, perPage, total))
}

func (h *DealHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateDealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	expectedCloseDate, err := parseDealDate(req.ExpectedCloseDate)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid expected_close_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
		return
	}

	deal, err := h.svc.Create(c.Request.Context(), scope, repository.CreateDealParams{
		PipelineID:        req.PipelineID,
		StageID:           req.StageID,
		CompanyID:         req.CompanyID,
		ContactID:         req.ContactID,
		Title:             req.Title,
		Value:             req.Value,
		Currency:          req.Currency,
		ExpectedCloseDate: expectedCloseDate,
		OwnerUserID:       req.OwnerUserID,
		CreatedBy:         userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrDealStageNotInPipeline) {
			corehttp.Fail(c, coreerrors.New("DEAL_STAGE_NOT_IN_PIPELINE", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	deal, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateDealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var expectedCloseDate *time.Time
	if req.ExpectedCloseDate != nil {
		parsed, err := parseDealDate(req.ExpectedCloseDate)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid expected_close_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
			return
		}
		expectedCloseDate = parsed
	}

	deal, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateDealParams{
		StageID:           req.StageID,
		CompanyID:         req.CompanyID,
		ContactID:         req.ContactID,
		Title:             req.Title,
		Value:             req.Value,
		Currency:          req.Currency,
		ExpectedCloseDate: expectedCloseDate,
		OwnerUserID:       req.OwnerUserID,
		UpdatedBy:         userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrDealStageNotInPipeline) {
			corehttp.Fail(c, coreerrors.New("DEAL_STAGE_NOT_IN_PIPELINE", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) Delete(c *gin.Context) {
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

func (h *DealHandler) Restore(c *gin.Context) {
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

func (h *DealHandler) MoveStage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.MoveDealStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	deal, err := h.svc.MoveStage(c.Request.Context(), scope, c.Param("id"), req.StageID, userID)
	if err != nil {
		if errors.Is(err, service.ErrDealStageNotInPipeline) {
			corehttp.Fail(c, coreerrors.New("DEAL_STAGE_NOT_IN_PIPELINE", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) CloseWon(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	deal, err := h.svc.CloseWon(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) CloseLost(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CloseLostDealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	deal, err := h.svc.CloseLost(c.Request.Context(), scope, c.Param("id"), req.LostReason, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}

func (h *DealHandler) ApproveDiscount(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.ApproveDealDiscountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	deal, err := h.svc.ApproveDiscount(c.Request.Context(), scope, c.Param("id"), req.DiscountPercent, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.DealFromDomain(deal))
}
