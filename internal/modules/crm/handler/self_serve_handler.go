package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

// SelfServePlatformScope me-resolve scope org platform; service bekerja di CRM platform, bukan di tenant pembeli.
type SelfServePlatformScope interface {
	PlatformScope(ctx context.Context) (coretenant.Scope, error)
}

// SelfServeHandler melayani checkout mandiri dari tenant customer. svc nil berarti konfigurasi runtime
// (bot, pipeline) belum siap: endpoint membalas 503 tetapi API lain tetap jalan.
type SelfServeHandler struct {
	svc      service.SelfServeService
	platform SelfServePlatformScope
	checker  permissionmiddleware.OrganizationPermissionChecker
}

func NewSelfServeHandler(svc service.SelfServeService, platform SelfServePlatformScope, checker permissionmiddleware.OrganizationPermissionChecker) *SelfServeHandler {
	return &SelfServeHandler{svc: svc, platform: platform, checker: checker}
}

func (h *SelfServeHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/app/self-serve")
	group.Use(middleware.RequireActiveTenant(), middleware.RequireCustomerTenant())
	group.POST("/checkout", permissionmiddleware.RequireOrganization(h.checker, "organization.billing.manage"), h.Checkout)
}

type selfServeCheckoutRequest struct {
	ProductID string `json:"product_id"`
}

type selfServeCheckoutResponse struct {
	InvoiceURL string `json:"invoice_url"`
	DealID     string `json:"deal_id"`
}

func (h *SelfServeHandler) Checkout(c *gin.Context) {
	if h.svc == nil {
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_NOT_CONFIGURED", "Pembelian mandiri belum dikonfigurasi.", http.StatusServiceUnavailable))
		return
	}
	var body selfServeCheckoutRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.ProductID == "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "product_id wajib diisi.", http.StatusUnprocessableEntity))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	platform, err := h.platform.PlatformScope(c.Request.Context())
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "self-serve: resolve platform scope failed", "error", err)
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_NOT_CONFIGURED", "Pembelian mandiri belum dikonfigurasi.", http.StatusServiceUnavailable))
		return
	}
	result, err := h.svc.Checkout(c.Request.Context(), platform, service.SelfServeCheckoutRequest{
		TenantOrganizationID: tenantContext.OrganizationID(),
		BuyerUserID:          permissionmiddleware.UserID(c),
		ProductID:            body.ProductID,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	corehttp.Created(c, "self-serve checkout ready", selfServeCheckoutResponse{InvoiceURL: result.InvoiceURL, DealID: result.DealID})
}

func (h *SelfServeHandler) fail(c *gin.Context, err error) {
	var step *service.SelfServeStepError
	switch {
	case errors.Is(err, service.ErrSelfServeProductUnavailable):
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_PRODUCT_UNAVAILABLE", "Paket ini tidak tersedia untuk dibeli langsung.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrSelfServeAlreadySubscribed):
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_ALREADY_SUBSCRIBED", "Workspace ini sudah berlangganan.", http.StatusConflict))
	case errors.Is(err, service.ErrSelfServeNotConfigured):
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_NOT_CONFIGURED", "Pembelian mandiri belum dikonfigurasi.", http.StatusServiceUnavailable))
	case errors.Is(err, service.ErrSelfServeInProgress):
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_IN_PROGRESS", "Pembayaran sedang disiapkan. Coba lagi sebentar.", http.StatusConflict))
	case errors.As(err, &step):
		// Error asli hanya di log; klien menerima nama langkah supaya bisa menampilkan pesan yang tepat.
		slog.ErrorContext(c.Request.Context(), "self-serve checkout step failed", "step", step.Step, "error", step.Err)
		response.ErrorWithData(c, http.StatusBadGateway, "SELF_SERVE_STEP_FAILED", "Pembayaran belum bisa disiapkan. Coba lagi.",
			map[string]any{"step": step.Step})
	default:
		corehttp.Fail(c, err)
	}
}
