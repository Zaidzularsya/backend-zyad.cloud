package handler

import (
	"context"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type PlatformInvoiceService interface {
	List(ctx context.Context, query dto.InvoiceListQuery) (dto.InvoiceListResponse, error)
	Create(ctx context.Context, request dto.CreateInvoiceRequest) (dto.InvoiceResponse, error)
}

type PlatformPaymentService interface {
	MarkInvoicePaidByID(
		ctx context.Context,
		invoiceID string,
		request dto.MarkInvoicePaidRequest,
	) (dto.PaymentResponse, error)
}

type PlatformBillingHandler struct {
	invoices PlatformInvoiceService
	payments PlatformPaymentService
	checker  permissionmiddleware.PermissionChecker
}

func NewPlatformBillingHandler(
	invoices PlatformInvoiceService,
	payments PlatformPaymentService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformBillingHandler {
	return &PlatformBillingHandler{
		invoices: invoices,
		payments: payments,
		checker:  checker,
	}
}

func (h *PlatformBillingHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/billing")
	group.Use(middleware.RequirePlatformTenant())

	group.GET("/invoices", permissionmiddleware.Require(h.checker, "platform.billing.invoice.read"), h.ListInvoices)
	group.POST("/invoices", permissionmiddleware.Require(h.checker, "platform.billing.invoice.manage"), h.CreateInvoice)
	group.POST("/invoices/:id/mark-paid", permissionmiddleware.Require(h.checker, "platform.billing.payment.manage"), h.MarkInvoicePaid)
}

func (h *PlatformBillingHandler) ListInvoices(c *gin.Context) {
	var query dto.InvoiceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.invoices.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing invoices retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformBillingHandler) CreateInvoice(c *gin.Context) {
	var request dto.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.invoices.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing invoice created successfully", result)
}

func (h *PlatformBillingHandler) MarkInvoicePaid(c *gin.Context) {
	var request dto.MarkInvoicePaidRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.payments.MarkInvoicePaidByID(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing invoice marked as paid successfully", result)
}
