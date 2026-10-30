package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/service"
)

// ActivationHandler serves the reseller's page activation invoice action.
// Entitlements (landing.enabled + landing.page_activation) are enforced by
// the service; the route only adds the page-update permission.
type ActivationHandler struct {
	svc service.ActivationService
}

func NewActivationHandler(svc service.ActivationService) *ActivationHandler {
	return &ActivationHandler{svc: svc}
}

func (h *ActivationHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/app/landing/pages")
	group.Use(middleware.RequireActiveTenant())
	group.POST("/:id/activation-invoice",
		permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.page.update"), h.CreateInvoice)
}

// CreateInvoice godoc
// @Summary Create the activation invoice of a landing page (reseller)
func (h *ActivationHandler) CreateInvoice(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	var req dto.CreateActivationInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Data invoice aktivasi tidak valid.", http.StatusUnprocessableEntity))
		return
	}
	res, err := h.svc.CreateInvoice(c.Request.Context(), scope, c.Param("id"), service.ActivationInput{
		BuyerName: req.BuyerName, BuyerContact: req.BuyerContact, BuyerEmail: req.BuyerEmail, Price: req.Price,
		DurationValue: req.DurationValue, DurationUnit: domain.ActivationDurationUnit(req.DurationUnit),
		InvoiceDueHours: req.InvoiceDueHours, InvoiceDueDays: req.InvoiceDueDays,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	a := res.Activation
	corehttp.Created(c, "Activation invoice created successfully", dto.ActivationInvoiceResponse{
		Activation: dto.ActivationResponse{
			ID: a.ID, PageID: a.PageID, InvoiceID: a.InvoiceID, BuyerName: a.BuyerName, BuyerContact: a.BuyerContact,
			BuyerEmail: a.BuyerEmail, Price: a.Price, DurationValue: a.DurationValue, DurationUnit: string(a.DurationUnit),
			InvoiceDueAt: a.InvoiceDueAt, Status: string(a.Status), PaidAt: a.PaidAt, CreatedAt: a.CreatedAt,
		},
		PreviewURL: res.PreviewURL,
		PaymentURL: res.PaymentURL,
	})
}
