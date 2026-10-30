package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/shared/response"
)

// ActivationInvoicesHandler serves the reseller's invoice dashboard: the list of activation invoices and
// the renewal of an open one's preview link. Entitlements for renewing are enforced by the service; the
// routes add the page-update permission, like creating the invoice.
type ActivationInvoicesHandler struct {
	svc service.ActivationManager
}

func NewActivationInvoicesHandler(svc service.ActivationManager) *ActivationInvoicesHandler {
	return &ActivationInvoicesHandler{svc: svc}
}

func (h *ActivationInvoicesHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/app/landing/activations")
	group.Use(middleware.RequireActiveTenant())
	perm := permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.page.update")
	group.GET("", perm, h.List)
	group.POST("/:id/renew", perm, h.Renew)
}

func queryInt(c *gin.Context, key string) int {
	n, _ := strconv.Atoi(c.Query(key))
	return n
}

// List godoc
// @Summary List the reseller's page activation invoices
func (h *ActivationInvoicesHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	res, err := h.svc.List(c.Request.Context(), scope, service.ActivationListQuery{
		Status: c.Query("status"), Page: queryInt(c, "page"), PerPage: queryInt(c, "per_page"),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items := make([]dto.ActivationInvoiceListItem, 0, len(res.Items))
	for _, e := range res.Items {
		a := e.Activation
		items = append(items, dto.ActivationInvoiceListItem{
			ID: a.ID, PageID: a.PageID, PageTitle: e.PageTitle, PageSlug: e.PageSlug, PageStatus: string(e.PageStatus),
			PageActiveUntil: e.PageActiveUntil, InvoiceID: a.InvoiceID, BuyerName: a.BuyerName, BuyerContact: a.BuyerContact,
			BuyerEmail: a.BuyerEmail, Price: a.Price, DurationValue: a.DurationValue, DurationUnit: string(a.DurationUnit),
			InvoiceDueAt: a.InvoiceDueAt, Status: string(a.Status), State: e.State, PaidAt: a.PaidAt,
			CreatedAt: a.CreatedAt, PaymentURL: e.PaymentURL,
		})
	}
	totalPages := 0
	if res.PerPage > 0 {
		totalPages = (int(res.Total) + res.PerPage - 1) / res.PerPage
	}
	c.Header("Cache-Control", "private, no-store")
	response.JSON(c, http.StatusOK, "Activation invoices retrieved successfully", dto.ActivationInvoiceListResponse{
		Items: items,
		Meta:  dto.PaginationMeta{Total: res.Total, Page: res.Page, PerPage: res.PerPage, TotalPages: totalPages},
	}, nil)
}

// Renew godoc
// @Summary Give an open activation a new preview link and a fresh preview window
func (h *ActivationInvoicesHandler) Renew(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	var req dto.RenewActivationRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Data perpanjangan tidak valid.", http.StatusUnprocessableEntity))
		return
	}
	res, err := h.svc.Renew(c.Request.Context(), scope, c.Param("id"), service.ActivationRenewInput{
		InvoiceDueHours: req.InvoiceDueHours, InvoiceDueDays: req.InvoiceDueDays,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	a := res.Activation
	c.Header("Cache-Control", "private, no-store")
	corehttp.OK(c, "Activation preview link renewed successfully", dto.ActivationInvoiceResponse{
		Activation: dto.ActivationResponse{
			ID: a.ID, PageID: a.PageID, InvoiceID: a.InvoiceID, BuyerName: a.BuyerName, BuyerContact: a.BuyerContact,
			BuyerEmail: a.BuyerEmail, Price: a.Price, DurationValue: a.DurationValue, DurationUnit: string(a.DurationUnit),
			InvoiceDueAt: a.InvoiceDueAt, Status: string(a.Status), PaidAt: a.PaidAt, CreatedAt: a.CreatedAt,
		},
		PreviewURL: res.PreviewURL,
		PaymentURL: res.PaymentURL,
	})
}
