package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
)

const publicIPLimitPerMinute = 30

// PublicInvoiceService adalah subset service yang dipakai halaman publik.
type PublicInvoiceService interface {
	View(ctx context.Context, token string) (service.PublicInvoiceView, error)
	PDF(ctx context.Context, token string) (service.InvoicePDF, error)
}

// PublicInvoiceHandler melayani link invoice tanpa login. Satu-satunya input yang
// mengidentifikasi dokumen adalah token; tenant diturunkan dari baris link.
type PublicInvoiceHandler struct {
	svc     PublicInvoiceService
	counter middleware.RateCounter
}

func NewPublicInvoiceHandler(svc PublicInvoiceService, counter middleware.RateCounter) *PublicInvoiceHandler {
	return &PublicInvoiceHandler{svc: svc, counter: counter}
}

// RegisterRoutes dipasang langsung pada api (di luar grup auth & tenant).
func (h *PublicInvoiceHandler) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/public/invoices")
	group.Use(
		func(c *gin.Context) { c.Header("X-Robots-Tag", "noindex"); c.Next() },
		middleware.RateLimit(h.counter, "rl:pubi:ip:", publicIPLimitPerMinute, time.Minute, func(c *gin.Context) string { return c.ClientIP() }),
	)
	group.GET("/:token", h.View)
	group.GET("/:token/pdf", h.PDF)
}

func failPublicInvoice(c *gin.Context, err error) {
	if errors.Is(err, service.ErrLinkInvalid) {
		corehttp.Fail(c, coreerrors.New("LINK_INVALID", "Link tidak valid atau sudah tidak berlaku.", http.StatusNotFound))
		return
	}
	// Detail hanya di log; path berisi token sehingga jangan dicatat.
	slog.ErrorContext(c.Request.Context(), "public invoice failed", "error", err)
	corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError))
}

func (h *PublicInvoiceHandler) View(c *gin.Context) {
	view, err := h.svc.View(c.Request.Context(), c.Param("token"))
	if err != nil {
		failPublicInvoice(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	corehttp.OK(c, "success", publicInvoiceFromView(view))
}

func (h *PublicInvoiceHandler) PDF(c *gin.Context) {
	pdf, err := h.svc.PDF(c.Request.Context(), c.Param("token"))
	if err != nil {
		failPublicInvoice(c, err)
		return
	}
	c.Header("Content-Disposition", `inline; filename="`+pdf.Filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdf.Content)
}

func publicInvoiceFromView(v service.PublicInvoiceView) dto.PublicInvoiceResponse {
	inv := v.Invoice
	out := dto.PublicInvoiceResponse{
		TenantName: v.TenantName, InvoiceNumber: inv.InvoiceNumber, Status: string(inv.Status), State: string(v.State),
		IssueDate: dto.DateString(inv.IssueDate), DueDate: dto.DateString(inv.DueDate),
		PeriodStart: dto.DateString(inv.PeriodStart), PeriodEnd: dto.DateString(inv.PeriodEnd),
		Currency: inv.Currency, GrandTotal: inv.GrandTotal, AmountPaid: inv.AmountPaid, Balance: inv.Balance(),
		Items: []dto.PublicInvoiceItem{}, CanPay: v.CanPay,
	}
	for _, it := range inv.Items {
		item := dto.PublicInvoiceItem{
			Description: it.Description, Quantity: it.Quantity, Unit: it.Unit, UnitPrice: it.UnitPrice, LineTotal: it.LineTotal,
			ChargeType: string(it.Pricing.ChargeType), PaymentTiming: string(it.Pricing.PaymentTiming),
			PeriodStart: dto.DateString(it.PeriodStart), PeriodEnd: dto.DateString(it.PeriodEnd),
		}
		if it.Pricing.Frequency != "" {
			f := string(it.Pricing.Frequency)
			item.BillingFrequency = &f
		}
		out.Items = append(out.Items, item)
	}
	return out
}
