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

type InvoiceHandler struct {
	svc service.InvoiceService
}

func NewInvoiceHandler(svc service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{svc: svc}
}

// RegisterRoutes registers invoice routes under the given parent group. See
// CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *InvoiceHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/invoices")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.delete"), h.Delete)
	group.POST("/:id/send", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.send"), h.Send)
	group.POST("/:id/mark-paid", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.mark_paid"), h.MarkPaid)
	group.POST("/:id/cancel", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.cancel"), h.Cancel)
}

func invoiceLineItemsFromRequest(requests []dto.LineItemRequest) []service.InvoiceLineInput {
	items := make([]service.InvoiceLineInput, 0, len(requests))
	for _, r := range requests {
		items = append(items, service.InvoiceLineInput{
			Description:     r.Description,
			Quantity:        r.Quantity,
			UnitPrice:       r.UnitPrice,
			DiscountPercent: r.DiscountPercent,
		})
	}
	return items
}

func (h *InvoiceHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.InvoiceListQuery
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

	invoices, total, err := h.svc.List(c.Request.Context(), scope, repository.InvoiceListFilter{
		Status:      domain.InvoiceStatus(query.Status),
		DealID:      query.DealID,
		ContactID:   query.ContactID,
		CompanyID:   query.CompanyID,
		QuotationID: query.QuotationID,
		Limit:       perPage,
		Offset:      (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.InvoiceListFromDomain(invoices), dto.BuildMeta(page, perPage, total))
}

func (h *InvoiceHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if req.QuotationID == "" && len(req.Items) == 0 {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "either quotation_id or items is required", http.StatusUnprocessableEntity))
		return
	}

	issueDate, err := parseDealDate(req.IssueDate)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid issue_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
		return
	}
	dueDate, err := parseDealDate(req.DueDate)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid due_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
		return
	}

	invoice, err := h.svc.Create(c.Request.Context(), scope, service.CreateInvoiceInput{
		QuotationID:   req.QuotationID,
		DealID:        req.DealID,
		ContactID:     req.ContactID,
		CompanyID:     req.CompanyID,
		InvoiceNumber: req.InvoiceNumber,
		IssueDate:     issueDate,
		DueDate:       dueDate,
		Currency:      req.Currency,
		TaxTotal:      req.TaxTotal,
		Items:         invoiceLineItemsFromRequest(req.Items),
		CreatedBy:     userID,
	})
	if err != nil {
		if err == service.ErrInvalidQuotationAmount {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}

func (h *InvoiceHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	invoice, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}

func (h *InvoiceHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var issueDate, dueDate *time.Time
	if req.IssueDate != nil {
		parsed, err := parseDealDate(req.IssueDate)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid issue_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
			return
		}
		issueDate = parsed
	}
	if req.DueDate != nil {
		parsed, err := parseDealDate(req.DueDate)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid due_date, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
			return
		}
		dueDate = parsed
	}

	invoice, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), service.UpdateInvoiceInput{
		QuotationID: req.QuotationID,
		DealID:      req.DealID,
		ContactID:   req.ContactID,
		CompanyID:   req.CompanyID,
		IssueDate:   issueDate,
		DueDate:     dueDate,
		UpdatedBy:   userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}

func (h *InvoiceHandler) Delete(c *gin.Context) {
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

func (h *InvoiceHandler) Send(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	invoice, err := h.svc.Send(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}

func (h *InvoiceHandler) MarkPaid(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.MarkInvoicePaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	invoice, err := h.svc.MarkPaid(c.Request.Context(), scope, c.Param("id"), req.AmountPaid, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}

func (h *InvoiceHandler) Cancel(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	invoice, err := h.svc.Cancel(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.InvoiceFromDomain(invoice))
}
