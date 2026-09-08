package handler

import (
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

type QuotationHandler struct {
	svc service.QuotationService
}

func NewQuotationHandler(svc service.QuotationService) *QuotationHandler {
	return &QuotationHandler{svc: svc}
}

// RegisterRoutes registers quotation routes under the given parent group.
// See CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *QuotationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/quotations")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.delete"), h.Delete)
	group.POST("/:id/send", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.send"), h.Send)
	group.POST("/:id/approve", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.approve"), h.Approve)
	group.POST("/:id/reject", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.reject"), h.Reject)
}

func lineItemsFromRequest(requests []dto.LineItemRequest) []service.QuotationLineInput {
	items := make([]service.QuotationLineInput, 0, len(requests))
	for _, r := range requests {
		items = append(items, service.QuotationLineInput{
			Description:     r.Description,
			Quantity:        r.Quantity,
			UnitPrice:       r.UnitPrice,
			DiscountPercent: r.DiscountPercent,
		})
	}
	return items
}

func (h *QuotationHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.QuotationListQuery
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

	quotations, total, err := h.svc.List(c.Request.Context(), scope, repository.QuotationListFilter{
		Status:    domain.QuotationStatus(query.Status),
		DealID:    query.DealID,
		ContactID: query.ContactID,
		CompanyID: query.CompanyID,
		Limit:     perPage,
		Offset:    (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.QuotationListFromDomain(quotations), dto.BuildMeta(page, perPage, total))
}

func (h *QuotationHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	validUntil, err := parseDealDate(req.ValidUntil)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid valid_until, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
		return
	}

	quotation, err := h.svc.Create(c.Request.Context(), scope, service.CreateQuotationInput{
		DealID:          req.DealID,
		ContactID:       req.ContactID,
		CompanyID:       req.CompanyID,
		QuotationNumber: req.QuotationNumber,
		ValidUntil:      validUntil,
		Currency:        req.Currency,
		Notes:           req.Notes,
		TaxTotal:        req.TaxTotal,
		Items:           lineItemsFromRequest(req.Items),
		CreatedBy:       userID,
	})
	if err != nil {
		if err == service.ErrInvalidQuotationAmount {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	quotation, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var validUntil *time.Time
	if req.ValidUntil != nil {
		parsed, err := parseDealDate(req.ValidUntil)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid valid_until, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
			return
		}
		validUntil = parsed
	}

	quotation, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), service.UpdateQuotationInput{
		DealID:     req.DealID,
		ContactID:  req.ContactID,
		CompanyID:  req.CompanyID,
		ValidUntil: validUntil,
		Notes:      req.Notes,
		UpdatedBy:  userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Delete(c *gin.Context) {
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

func (h *QuotationHandler) Send(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	quotation, err := h.svc.Send(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Approve(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	quotation, err := h.svc.Approve(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Reject(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	quotation, err := h.svc.Reject(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}
