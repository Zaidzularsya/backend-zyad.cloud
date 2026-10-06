package handler

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
	"zyad.cloud/internal/shared/response"
)

type InvoiceHandler struct {
	invoices service.InvoiceService
	sends    service.SendService
	perm     permissionmiddleware.CombinedPermissionChecker
}

func NewInvoiceHandler(invoices service.InvoiceService, sends service.SendService) *InvoiceHandler {
	return &InvoiceHandler{invoices: invoices, sends: sends}
}

func (h *InvoiceHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	group := router.Group("/invoices")
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Get)
	group.PUT("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.update"), h.Update)
	// Menerbitkan juga mengirim invoice ke pelanggan, jadi butuh izin kirim.
	group.POST("/:id/issue", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.send"), h.Issue)
	group.POST("/:id/void", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.void"), h.Void)
	group.GET("/:id/pdf", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.PDF)
	group.POST("/:id/link", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.send"), h.Link)
	group.POST("/:id/send", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.send"), h.Send)
	group.GET("/:id/sends", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Sends)
}

// inputFromRequest memetakan body HTTP ke InvoiceInput manual. Semua id divalidasi sebagai UUID
// di sini (batas HTTP); sumber/kunci idempotensi tidak pernah berasal dari HTTP.
func inputFromRequest(req dto.InvoiceRequest) (service.InvoiceInput, string) {
	if !validUUID(req.AccountID) {
		return service.InvoiceInput{}, "Pelanggan tidak valid."
	}
	if req.PICUserID != "" && !validUUID(req.PICUserID) {
		return service.InvoiceInput{}, "PIC tidak valid."
	}
	periodStart, ok := parseDate(req.PeriodStart)
	if !ok {
		return service.InvoiceInput{}, "Tanggal periode tidak valid. Gunakan format YYYY-MM-DD."
	}
	periodEnd, ok := parseDate(req.PeriodEnd)
	if !ok {
		return service.InvoiceInput{}, "Tanggal periode tidak valid. Gunakan format YYYY-MM-DD."
	}
	in := service.InvoiceInput{
		AccountID: req.AccountID, Channels: req.Channels, PICUserID: req.PICUserID, Notes: req.Notes, Currency: req.Currency,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
	}
	for _, it := range req.Items {
		if it.ProductID != "" && !validUUID(it.ProductID) {
			return service.InvoiceInput{}, "Produk pada baris tidak valid."
		}
		start, ok1 := parseDate(it.PeriodStart)
		end, ok2 := parseDate(it.PeriodEnd)
		if !ok1 || !ok2 {
			return service.InvoiceInput{}, "Tanggal periode baris tidak valid. Gunakan format YYYY-MM-DD."
		}
		in.Lines = append(in.Lines, service.LineInput{
			LineInput: pricing.LineInput{
				Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice, DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent,
				Attrs: pricing.Attributes{ChargeType: pricing.ChargeType(it.ChargeType), Frequency: pricing.Frequency(it.BillingFrequency), PaymentTiming: pricing.PaymentTiming(it.PaymentTiming)},
			},
			Unit: it.Unit, ProductID: it.ProductID, SKU: it.SKU, PeriodStart: start, PeriodEnd: end,
		})
	}
	return in, ""
}

// parseDate: "" → (nil, true); YYYY-MM-DD → tanggal kalender (00:00 UTC); selain itu (nil, false).
func parseDate(v string) (*time.Time, bool) {
	v = trim(v)
	if v == "" {
		return nil, true
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, false
	}
	return &d, true
}

var knownStatuses = map[string]bool{"draft": true, "issued": true, "paid": true, "overdue": true, "void": true}
var knownSources = map[string]bool{"sales_order": true, "contract": true, "manual": true}

func (h *InvoiceHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var q dto.InvoiceListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	if (q.Status != "" && !knownStatuses[q.Status]) || (q.SourceType != "" && !knownSources[q.SourceType]) {
		validationFail(c, "Filter status atau sumber tidak dikenal.")
		return
	}
	for _, id := range []string{q.AccountID, q.SourceID, q.ContractID} {
		if id != "" && !validUUID(id) {
			validationFail(c, "Filter id tidak valid.")
			return
		}
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	invoices, total, err := h.invoices.List(c.Request.Context(), scope, repository.InvoiceListFilter{
		Status: q.Status, AccountID: q.AccountID, SourceType: q.SourceType, SourceID: q.SourceID, ContractID: q.ContractID,
		SendFailed: q.SendFailed, Search: q.Search, Limit: perPage, Offset: (page - 1) * perPage,
	})
	if err != nil {
		failReceivable(c, err)
		return
	}
	ids := make([]string, len(invoices))
	for i, inv := range invoices {
		ids[i] = inv.ID
	}
	last, err := h.sends.LatestByInvoices(c.Request.Context(), scope, ids)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.InvoiceResponse, 0, len(invoices))
	for _, inv := range invoices {
		var ls *domain.Send
		if s, ok := last[inv.ID]; ok {
			ls = &s
		}
		out = append(out, dto.InvoiceFromDomain(inv, ls))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *InvoiceHandler) Create(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.InvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	in, problem := inputFromRequest(req)
	if problem != "" {
		validationFail(c, problem)
		return
	}
	inv, err := h.invoices.CreateDraft(c.Request.Context(), scope, in, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, "success", dto.InvoiceFromDomain(inv, nil), nil)
}

func (h *InvoiceHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	inv, err := h.invoices.Get(c.Request.Context(), scope, id)
	if err != nil {
		failReceivable(c, err)
		return
	}
	var last *domain.Send
	if sends, err := h.sends.List(c.Request.Context(), scope, id); err == nil && len(sends) > 0 {
		last = &sends[0]
	}
	corehttp.OK(c, "success", dto.InvoiceFromDomain(inv, last))
}

func (h *InvoiceHandler) Update(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	var req dto.InvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	in, problem := inputFromRequest(req)
	if problem != "" {
		validationFail(c, problem)
		return
	}
	inv, err := h.invoices.UpdateDraft(c.Request.Context(), scope, id, in, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.InvoiceFromDomain(inv, nil))
}

func (h *InvoiceHandler) Issue(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	inv, err := h.invoices.Issue(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.InvoiceFromDomain(inv, nil))
}

func (h *InvoiceHandler) Void(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	var req dto.VoidInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	inv, err := h.invoices.Void(c.Request.Context(), scope, id, req.Reason, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.InvoiceFromDomain(inv, nil))
}

func (h *InvoiceHandler) PDF(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	pdf, err := h.invoices.PDF(c.Request.Context(), scope, id)
	if err != nil {
		failInvoiceDocument(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+pdf.Filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdf.Content)
}

func (h *InvoiceHandler) Link(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	url, expires, err := h.invoices.Link(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failInvoiceDocument(c, err)
		return
	}
	corehttp.OK(c, "success", dto.InvoiceLinkResponse{URL: url, ExpiresAt: expires})
}

func (h *InvoiceHandler) Send(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	var req dto.SendInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	need := "email.send"
	switch req.Channel {
	case "email":
	case "whatsapp":
		need = "whatsapp.message.send"
	default:
		validationFail(c, "Kanal harus email atau whatsapp.")
		return
	}
	if !allowed(h.perm, c, need) {
		forbidden(c)
		return
	}
	if trim(req.ClientRequestID) == "" || len(req.ClientRequestID) > 100 {
		validationFail(c, "client_request_id wajib diisi (maksimal 100 karakter).")
		return
	}
	if utf8.RuneCountInString(req.Message) > 2000 || len(req.Recipient) > 255 {
		validationFail(c, "Pesan maksimal 2000 karakter dan penerima maksimal 255 karakter.")
		return
	}
	send, err := h.sends.Send(c.Request.Context(), scope, id, service.SendInput{
		Channel: req.Channel, Recipient: req.Recipient, Message: req.Message, ClientRequestID: req.ClientRequestID,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SendFromDomain(send))
}

func (h *InvoiceHandler) Sends(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	sends, err := h.sends.List(c.Request.Context(), scope, id)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.InvoiceSendResponse, 0, len(sends))
	for _, s := range sends {
		out = append(out, dto.SendFromDomain(s))
	}
	corehttp.OK(c, "success", out)
}
