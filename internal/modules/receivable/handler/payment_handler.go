package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/businesstime"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

type PaymentHandler struct{ svc service.PaymentService }

func NewPaymentHandler(svc service.PaymentService) *PaymentHandler { return &PaymentHandler{svc: svc} }

func (h *PaymentHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/payments", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.ListAll)
	router.GET("/invoices/:id/payments", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.List)
	router.POST("/invoices/:id/payments", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.mark_paid"), h.Record)
}

func (h *PaymentHandler) ListAll(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var q dto.PaymentListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	list, total, err := h.svc.ListAll(c.Request.Context(), scope, perPage, (page-1)*perPage)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.PaymentResponse, 0, len(list))
	for _, p := range list {
		out = append(out, dto.PaymentFromDomain(p))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *PaymentHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	list, err := h.svc.List(c.Request.Context(), scope, id)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.PaymentResponse, 0, len(list))
	for _, p := range list {
		out = append(out, dto.PaymentFromDomain(p))
	}
	corehttp.OK(c, "success", out)
}

func (h *PaymentHandler) Record(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	var req dto.RecordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	paidAt, err := parsePaidAt(req.PaidAt)
	if err != nil {
		validationFail(c, "Tanggal bayar tidak valid. Gunakan format RFC3339 atau YYYY-MM-DD.")
		return
	}
	inv, pay, err := h.svc.RecordManual(c.Request.Context(), scope, id, service.ManualPaymentInput{
		Amount: req.Amount, PaidAt: paidAt, Reference: req.Reference, Note: req.Note,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RecordPaymentResponse{Invoice: dto.InvoiceFromDomain(inv, nil), Payment: dto.PaymentFromDomain(pay)})
}

// parsePaidAt menerima RFC3339 atau tanggal saja. Tanggal saja berarti awal hari itu di WIB
// (bukan 00:00 UTC), supaya pembayaran "hari ini" yang dicatat dini hari WIB tidak dianggap masa depan.
func parsePaidAt(v string) (time.Time, error) {
	v = trim(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, err
	}
	return businesstime.DayStartUTC(d), nil
}
