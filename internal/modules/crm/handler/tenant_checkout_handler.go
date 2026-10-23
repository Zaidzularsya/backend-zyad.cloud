package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/service"
)

// Batas laju checkout publik tenant. Satu checkout bisa membuat user + workspace + rantai CRM,
// jadi batasnya jauh lebih ketat daripada endpoint publik baca. Batas IP dievaluasi dulu agar
// satu IP tidak menghabiskan kuota tenant.
const (
	tenantCheckoutIPLimit      = 10
	tenantCheckoutIPWindow     = 10 * time.Minute
	tenantCheckoutTenantLimit  = 100
	tenantCheckoutTenantWindow = time.Hour
	tenantCheckoutUAMaxLength  = 500
)

// TenantCheckoutHandler melayani POST /public/tenant-checkout. Tenant SELALU berasal dari host
// (middleware.ResolvePublicOrganization pada grup publik); organization_id di body/query diabaikan.
type TenantCheckoutHandler struct {
	svc     service.TenantCheckoutService
	counter middleware.RateCounter
}

func NewTenantCheckoutHandler(svc service.TenantCheckoutService, counter middleware.RateCounter) *TenantCheckoutHandler {
	return &TenantCheckoutHandler{svc: svc, counter: counter}
}

// RegisterRoutes WAJIB dipasang pada grup publik yang me-resolve tenant dari host.
func (h *TenantCheckoutHandler) RegisterRoutes(router gin.IRoutes) {
	perIP := middleware.RateLimit(h.counter, "rl:tcheckout:ip:", tenantCheckoutIPLimit, tenantCheckoutIPWindow,
		func(c *gin.Context) string { return c.ClientIP() })
	perTenant := middleware.RateLimit(h.counter, "rl:tcheckout:org:", tenantCheckoutTenantLimit, tenantCheckoutTenantWindow,
		func(c *gin.Context) string {
			if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
				return tc.OrganizationID()
			}
			return "unknown"
		})
	router.POST("/public/tenant-checkout", perIP, perTenant, h.Checkout)
}

type tenantCheckoutRequest struct {
	GoogleCredential string `json:"google_credential"`
	ProductID        string `json:"product_id"`
}

type tenantCheckoutResponse struct {
	InvoiceURL string `json:"invoice_url"`
}

func (h *TenantCheckoutHandler) Checkout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	tc, err := coretenant.RequireContext(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	// Host platform memakai /app/self-serve/checkout.
	if tc.OrganizationType() == coretenant.OrganizationTypePlatform {
		corehttp.Fail(c, coreerrors.New("TENANT_CHECKOUT_NOT_FOUND", "checkout tenant tidak ditemukan untuk host ini", http.StatusNotFound))
		return
	}
	var body tenantCheckoutRequest
	if err := c.ShouldBindJSON(&body); err != nil ||
		strings.TrimSpace(body.GoogleCredential) == "" || strings.TrimSpace(body.ProductID) == "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "google_credential dan product_id wajib diisi.", http.StatusUnprocessableEntity))
		return
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	ua := c.Request.UserAgent()
	if len(ua) > tenantCheckoutUAMaxLength {
		ua = ua[:tenantCheckoutUAMaxLength]
	}
	result, err := h.svc.Checkout(c.Request.Context(), scope, service.TenantCheckoutInput{
		GoogleCredential: body.GoogleCredential, ProductID: strings.TrimSpace(body.ProductID),
		IP: c.ClientIP(), UserAgent: ua,
	})
	if err != nil {
		failSelfServe(c, err)
		return
	}
	// DealID adalah data CRM internal tenant: tidak dikembalikan ke pembeli publik.
	corehttp.Created(c, "tenant checkout ready", tenantCheckoutResponse{InvoiceURL: result.InvoiceURL})
}
