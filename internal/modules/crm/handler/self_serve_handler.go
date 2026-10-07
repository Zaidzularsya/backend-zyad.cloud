package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/businesstime"
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

	subscriptions SubscriptionProvider
	graceDays     int
}

// SubscriptionProvider membangun ringkasan langganan workspace (service.TenantAccess).
type SubscriptionProvider interface {
	Subscription(ctx context.Context, platform coretenant.Scope, tenantOrganizationID string, today time.Time, graceDays int) (service.SubscriptionView, error)
}

// WithSubscription mengaktifkan GET /app/self-serve/subscription.
func (h *SelfServeHandler) WithSubscription(p SubscriptionProvider, graceDays int) *SelfServeHandler {
	h.subscriptions, h.graceDays = p, graceDays
	return h
}

func NewSelfServeHandler(svc service.SelfServeService, platform SelfServePlatformScope, checker permissionmiddleware.OrganizationPermissionChecker) *SelfServeHandler {
	return &SelfServeHandler{svc: svc, platform: platform, checker: checker}
}

func (h *SelfServeHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/app/self-serve")
	group.Use(middleware.RequireActiveTenant(), middleware.RequireCustomerTenant())
	group.POST("/checkout", permissionmiddleware.RequireOrganization(h.checker, "organization.billing.manage"), h.Checkout)
	group.GET("/subscription", permissionmiddleware.RequireOrganization(h.checker, "organization.billing.read"), h.Subscription)
}

type subscriptionProductResponse struct {
	Name      string `json:"name"`
	SKU       string `json:"sku"`
	Frequency string `json:"frequency"`
}

type subscriptionFeatureResponse struct {
	FeatureKey string          `json:"feature_key"`
	Value      json.RawMessage `json:"value"`
	Label      string          `json:"label"`
}

type subscriptionInvoiceResponse struct {
	Number      string `json:"number"`
	Status      string `json:"status"`
	Total       string `json:"total"`
	PeriodLabel string `json:"period_label"`
	DueDate     string `json:"due_date"`
	URL         string `json:"url"`
}

type subscriptionResponse struct {
	Status          string                        `json:"status"`
	Product         *subscriptionProductResponse  `json:"product"`
	ContractNumber  string                        `json:"contract_number"`
	NextInvoiceDate string                        `json:"next_invoice_date"`
	OverdueDays     int                           `json:"overdue_days"`
	SuspendInDays   *int                          `json:"suspend_in_days"`
	Features        []subscriptionFeatureResponse `json:"features"`
	Invoices        []subscriptionInvoiceResponse `json:"invoices"`
}

func dateString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// Subscription membaca langganan workspace pemanggil. Workspace selalu diambil dari konteks tenant,
// bukan dari parameter, sehingga satu tenant tidak bisa membaca data tenant lain.
func (h *SelfServeHandler) Subscription(c *gin.Context) {
	if h.subscriptions == nil {
		corehttp.Fail(c, coreerrors.New("SELF_SERVE_NOT_CONFIGURED", "Pembelian mandiri belum dikonfigurasi.", http.StatusServiceUnavailable))
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
	view, err := h.subscriptions.Subscription(c.Request.Context(), platform, tenantContext.OrganizationID(), businesstime.DayOf(time.Now()), h.graceDays)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "self-serve: subscription failed", "error", err)
		corehttp.Fail(c, coreerrors.New("SUBSCRIPTION_UNAVAILABLE", "Data langganan belum bisa dimuat. Coba lagi.", http.StatusBadGateway))
		return
	}
	out := subscriptionResponse{
		Status: view.Status, ContractNumber: view.ContractNumber, NextInvoiceDate: dateString(view.NextInvoiceDate),
		OverdueDays: view.OverdueDays, SuspendInDays: view.SuspendInDays,
		Features: make([]subscriptionFeatureResponse, 0, len(view.Features)),
		Invoices: make([]subscriptionInvoiceResponse, 0, len(view.Invoices)),
	}
	if view.Product != nil {
		out.Product = &subscriptionProductResponse{Name: view.Product.Name, SKU: view.Product.SKU, Frequency: view.Product.Frequency}
	}
	for _, f := range view.Features {
		out.Features = append(out.Features, subscriptionFeatureResponse{FeatureKey: f.FeatureKey, Value: f.Value, Label: f.Label})
	}
	for _, inv := range view.Invoices {
		out.Invoices = append(out.Invoices, subscriptionInvoiceResponse{
			Number: inv.Number, Status: inv.Status, Total: inv.Total, PeriodLabel: inv.PeriodLabel, DueDate: dateString(inv.DueDate), URL: inv.URL,
		})
	}
	corehttp.OK(c, "subscription loaded", out)
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
