package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
)

const (
	publicIPLimitPerMinute   = 30
	publicTokenPostsPerHour  = 10
	publicUserAgentMaxLength = 500
)

// PublicQuotationService adalah subset service yang dipakai halaman publik.
type PublicQuotationService interface {
	View(ctx context.Context, token string) (service.PublicQuotationView, error)
	PDF(ctx context.Context, token string) (service.QuotationPDF, error)
	Approve(ctx context.Context, token string, in service.PublicResponseInput) (service.PublicQuotationView, error)
	RequestRevision(ctx context.Context, token string, in service.PublicResponseInput) (service.PublicQuotationView, error)
}

// PublicQuotationHandler melayani link penawaran tanpa login. Satu-satunya
// input yang mengidentifikasi dokumen adalah token; tenant diturunkan dari
// baris link, bukan dari request.
type PublicQuotationHandler struct {
	svc     PublicQuotationService
	counter middleware.RateCounter
}

func NewPublicQuotationHandler(svc PublicQuotationService, counter middleware.RateCounter) *PublicQuotationHandler {
	return &PublicQuotationHandler{svc: svc, counter: counter}
}

// RegisterRoutes dipasang langsung pada api (di luar grup auth & tenant).
func (h *PublicQuotationHandler) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/public/quotations")
	group.Use(
		func(c *gin.Context) { c.Header("X-Robots-Tag", "noindex"); c.Next() },
		middleware.RateLimit(h.counter, "rl:pubq:ip:", publicIPLimitPerMinute, time.Minute, func(c *gin.Context) string { return c.ClientIP() }),
	)
	perToken := middleware.RateLimit(h.counter, "rl:pubq:tok:", publicTokenPostsPerHour, time.Hour, func(c *gin.Context) string {
		sum := sha256.Sum256([]byte(c.Param("token")))
		return hex.EncodeToString(sum[:])
	})

	group.GET("/:token", h.View)
	group.GET("/:token/pdf", h.PDF)
	group.POST("/:token/approve", perToken, h.Approve)
	group.POST("/:token/revision", perToken, h.RequestRevision)
}

func failPublicQuotation(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrLinkInvalid):
		corehttp.Fail(c, coreerrors.New("LINK_INVALID", "Link tidak valid atau sudah tidak berlaku.", http.StatusNotFound))
	case errors.Is(err, service.ErrQuotationNotRespondable):
		corehttp.Fail(c, coreerrors.New("QUOTATION_NOT_RESPONDABLE", "Penawaran ini sudah tidak menunggu respons.", http.StatusConflict))
	case errors.Is(err, service.ErrInvalidResponse):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
	default:
		// Detail hanya di log; path berisi token sehingga jangan dicatat.
		slog.ErrorContext(c.Request.Context(), "public quotation failed", "error", err)
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError))
	}
}

func (h *PublicQuotationHandler) View(c *gin.Context) {
	view, err := h.svc.View(c.Request.Context(), c.Param("token"))
	if err != nil {
		failPublicQuotation(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	corehttp.OK(c, "success", publicQuotationFromView(view))
}

func (h *PublicQuotationHandler) PDF(c *gin.Context) {
	pdf, err := h.svc.PDF(c.Request.Context(), c.Param("token"))
	if err != nil {
		failPublicQuotation(c, err)
		return
	}
	c.Header("Content-Disposition", `inline; filename="`+pdf.Filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdf.Content)
}

func (h *PublicQuotationHandler) Approve(c *gin.Context) {
	var req dto.PublicApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.Agree {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "responder_name dan persetujuan (agree) wajib diisi", http.StatusUnprocessableEntity))
		return
	}
	view, err := h.svc.Approve(c.Request.Context(), c.Param("token"), service.PublicResponseInput{
		ResponderName: req.ResponderName, IP: c.ClientIP(), UserAgent: userAgent(c),
	})
	if err != nil {
		failPublicQuotation(c, err)
		return
	}
	corehttp.OK(c, "success", publicQuotationFromView(view))
}

func (h *PublicQuotationHandler) RequestRevision(c *gin.Context) {
	var req dto.PublicRevisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "responder_name dan categories wajib diisi", http.StatusUnprocessableEntity))
		return
	}
	categories := make([]domain.RevisionCategory, 0, len(req.Categories))
	for _, code := range req.Categories {
		categories = append(categories, domain.RevisionCategory(code))
	}
	view, err := h.svc.RequestRevision(c.Request.Context(), c.Param("token"), service.PublicResponseInput{
		ResponderName: req.ResponderName, Note: req.Note, Categories: categories, IP: c.ClientIP(), UserAgent: userAgent(c),
	})
	if err != nil {
		failPublicQuotation(c, err)
		return
	}
	corehttp.OK(c, "success", publicQuotationFromView(view))
}

func userAgent(c *gin.Context) string {
	ua := c.GetHeader("User-Agent")
	if len(ua) > publicUserAgentMaxLength {
		return ua[:publicUserAgentMaxLength]
	}
	return ua
}

func publicQuotationFromView(v service.PublicQuotationView) dto.PublicQuotationResponse {
	q := v.Quotation
	out := dto.PublicQuotationResponse{
		TenantName: v.TenantName, QuotationNumber: v.QuotationNumber, Status: string(v.Status), State: string(v.State),
		ValidUntil: v.ValidUntil, Currency: q.Currency, GrandTotal: q.GrandTotal, OneTimeTotal: q.OneTimeTotal,
		FirstInvoiceTotal: q.FirstInvoiceTotal, RecurringTotals: map[string]string{}, Items: []dto.PublicQuotationItem{},
	}
	for k, val := range q.RecurringTotals {
		out.RecurringTotals[string(k)] = val
	}
	for _, it := range q.Items {
		item := dto.PublicQuotationItem{Description: it.Description, Quantity: it.Quantity, Unit: it.Unit,
			UnitPrice: it.UnitPrice, LineTotal: it.LineTotal}
		item.ChargeType = string(it.Pricing.ChargeType)
		if it.Pricing.Frequency != "" {
			f := string(it.Pricing.Frequency)
			item.BillingFrequency = &f
		}
		item.PaymentTiming = string(it.Pricing.PaymentTiming)
		for _, f := range it.Features {
			if f.Label != "" {
				item.Features = append(item.Features, f.Label)
			}
		}
		out.Items = append(out.Items, item)
	}
	if r := v.LastResponse; r != nil {
		last := &dto.PublicQuotationLastResponse{Action: string(r.Action), ResponderName: r.ResponderName, CreatedAt: r.CreatedAt}
		for _, c := range r.Categories {
			last.Categories = append(last.Categories, string(c))
		}
		out.LastResponse = last
	}
	return out
}
