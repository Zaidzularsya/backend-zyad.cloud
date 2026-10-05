package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/pricing"
	"zyad.cloud/internal/shared/response"
)

type QuotationHandler struct {
	svc  service.QuotationService
	perm permissionmiddleware.CombinedPermissionChecker
}

func NewQuotationHandler(svc service.QuotationService) *QuotationHandler {
	return &QuotationHandler{svc: svc}
}

// RegisterRoutes registers quotation routes under the given parent group.
// See CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
func (h *QuotationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	group := router.Group("/quotations")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.delete"), h.Delete)
	group.POST("/:id/send", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.send"), h.Send)
	group.POST("/:id/approve", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.approve"), h.Approve)
	group.POST("/:id/reject", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.reject"), h.Reject)
	group.POST("/:id/revise", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.update"), h.Revise)
	group.GET("/:id/pdf", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.PDF)
	group.GET("/:id/summary", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.Summary)
	group.GET("/:id/sends", permissionmiddleware.RequireOrganizationOrGlobal(p, "quotation.read"), h.Sends)
}

// allowed: cek permission tambahan di dalam handler (mis. izin kanal kirim).
func (h *QuotationHandler) allowed(c *gin.Context, perms ...string) bool {
	if h.perm == nil || len(perms) == 0 {
		return true
	}
	ctx := c.Request.Context()
	userID := permissionmiddleware.UserID(c)
	if tc, ok := coretenant.FromContext(ctx); ok {
		if h.perm.CanOrganization(ctx, userID, tc.OrganizationID(), perms) == nil {
			return true
		}
	}
	return h.perm.Can(ctx, userID, perms) == nil
}

func failQuotation(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrChannelUnavailable):
		corehttp.Fail(c, coreerrors.New("CHANNEL_UNAVAILABLE", err.Error(), http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrQuotationNotSendable):
		corehttp.Fail(c, coreerrors.New("QUOTATION_NOT_SENDABLE", "Quotation dengan status ini tidak bisa dikirim.", http.StatusConflict))
	case errors.Is(err, service.ErrInvalidSendInput):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrQuotationLocked):
		corehttp.Fail(c, coreerrors.New("QUOTATION_LOCKED", "Quotation sudah terkirim. Buat revisi untuk mengubah.", http.StatusConflict))
	case errors.Is(err, service.ErrQuotationNotRevisable):
		corehttp.Fail(c, coreerrors.New("QUOTATION_NOT_REVISABLE", err.Error(), http.StatusConflict))
	case errors.Is(err, service.ErrProductInactive):
		corehttp.Fail(c, coreerrors.New("PRODUCT_INACTIVE", "Produk tidak aktif atau sudah dihapus.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrQuotationDealNotFound):
		corehttp.Fail(c, coreerrors.New("QUOTATION_DEAL_NOT_FOUND", "Deal tidak ditemukan.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrInvalidQuotationAmount):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "Item tidak valid: deskripsi wajib, qty/harga ≥ 0, diskon & pajak 0–100; charge_type one_time|recurring, billing_frequency wajib hanya untuk recurring, payment_timing prepaid|postpaid.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrQuotationDocumentsUnavailable):
		corehttp.Fail(c, coreerrors.New("QUOTATION_PDF_FAILED", "PDF tidak dapat dibuat.", http.StatusBadGateway))
	default:
		corehttp.Fail(c, err)
	}
}

// failQuotationDocument: error render/storage yang bukan AppError atau sentinel
// quotation dilaporkan sebagai QUOTATION_PDF_FAILED (detail hanya di log).
func failQuotationDocument(c *gin.Context, err error) {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) || errors.Is(err, service.ErrChannelUnavailable) ||
		errors.Is(err, service.ErrQuotationNotSendable) || errors.Is(err, service.ErrInvalidSendInput) ||
		errors.Is(err, service.ErrQuotationLocked) || errors.Is(err, service.ErrQuotationNotRevisable) ||
		errors.Is(err, service.ErrProductInactive) || errors.Is(err, service.ErrQuotationDealNotFound) ||
		errors.Is(err, service.ErrInvalidQuotationAmount) || errors.Is(err, service.ErrQuotationDocumentsUnavailable) {
		failQuotation(c, err)
		return
	}
	slog.ErrorContext(c.Request.Context(), "quotation pdf failed", "quotation_id", c.Param("id"), "error", err)
	corehttp.Fail(c, coreerrors.New("QUOTATION_PDF_FAILED", "PDF tidak dapat dibuat.", http.StatusBadGateway))
}

func lineItemsFromRequest(requests []dto.LineItemRequest) []service.QuotationLineInput {
	items := make([]service.QuotationLineInput, 0, len(requests))
	for _, r := range requests {
		items = append(items, service.QuotationLineInput{
			Description:     r.Description,
			Quantity:        r.Quantity,
			UnitPrice:       r.UnitPrice,
			DiscountPercent: r.DiscountPercent,
			ProductID:       r.ProductID,
			TaxPercent:      r.TaxPercent,
			Unit:            r.Unit,
			Pricing: pricing.Attributes{
				ChargeType:    pricing.ChargeType(r.ChargeType),
				Frequency:     pricing.Frequency(r.BillingFrequency),
				PaymentTiming: pricing.PaymentTiming(r.PaymentTiming),
			},
		})
	}
	return items
}

func (h *QuotationHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.QuotationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	quotations, total, err := h.svc.List(c.Request.Context(), scope, repository.QuotationListFilter{
		Status:    domain.QuotationStatus(query.Status),
		DealID:    query.DealID,
		ContactID: query.ContactID,
		CompanyID: query.CompanyID,
		Limit:     perPage,
		Offset:    (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.QuotationListFromDomain(quotations), dto.BuildMeta(page, perPage, total))
}

func (h *QuotationHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	validUntil, err := parseDealDate(req.ValidUntil)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid valid_until, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
		return
	}

	quotation, err := h.svc.Create(c.Request.Context(), scope, service.CreateQuotationInput{
		DealID:          req.DealID,
		ContactID:       req.ContactID,
		CompanyID:       req.CompanyID,
		QuotationNumber: req.QuotationNumber,
		ValidUntil:      validUntil,
		Currency:        req.Currency,
		Notes:           req.Notes,
		TaxTotal:        req.TaxTotal,
		Items:           lineItemsFromRequest(req.Items),
		CreatedBy:       userID,
	})
	if err != nil {
		failQuotation(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	quotation, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var validUntil *time.Time
	if req.ValidUntil != nil {
		parsed, err := parseDealDate(req.ValidUntil)
		if err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid valid_until, expected YYYY-MM-DD", http.StatusUnprocessableEntity))
			return
		}
		validUntil = parsed
	}

	var items []service.QuotationLineInput
	if req.Items != nil {
		items = lineItemsFromRequest(req.Items)
	}

	quotation, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), service.UpdateQuotationInput{
		DealID:     req.DealID,
		ContactID:  req.ContactID,
		CompanyID:  req.CompanyID,
		ValidUntil: validUntil,
		Notes:      req.Notes,
		UpdatedBy:  userID,
		Items:      items,
	})
	if err != nil {
		failQuotation(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		failQuotation(c, err)
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *QuotationHandler) Send(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.SendQuotationRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
	}
	if req.Channel == "email" || req.Channel == "whatsapp" {
		h.sendVia(c, scope, userID, req)
		return
	}
	quotation, err := h.svc.MarkSentManually(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		failQuotationDocument(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) Approve(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	quotation, err := h.svc.Approve(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		failQuotation(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationDecisionResponse{
		QuotationResponse: dto.QuotationFromDomain(quotation),
		SuggestDealStatus: service.SuggestDealStatusAfterApprove(quotation),
	})
}

func (h *QuotationHandler) Reject(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	quotation, err := h.svc.Reject(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		failQuotation(c, err)
		return
	}

	corehttp.OK(c, "success", dto.QuotationDecisionResponse{QuotationResponse: dto.QuotationFromDomain(quotation)})
}

func (h *QuotationHandler) Revise(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	quotation, err := h.svc.Revise(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		failQuotation(c, err)
		return
	}
	corehttp.OK(c, "success", dto.QuotationFromDomain(quotation))
}

func (h *QuotationHandler) PDF(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	pdf, err := h.svc.PDF(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failQuotationDocument(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+pdf.Filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdf.Content)
}

func (h *QuotationHandler) sendVia(c *gin.Context, scope coretenant.Scope, userID string, req dto.SendQuotationRequest) {
	need := "email.send"
	if req.Channel == "whatsapp" {
		need = "whatsapp.message.send"
	}
	if !h.allowed(c, need) {
		corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden))
		return
	}
	if req.Mode == "" || req.ClientRequestID == "" {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "mode and client_request_id are required", http.StatusUnprocessableEntity))
		return
	}
	res, err := h.svc.SendVia(c.Request.Context(), scope, c.Param("id"), service.SendQuotationInput{
		Channel:            domain.QuotationSendChannel(req.Channel),
		Mode:               domain.QuotationSendMode(req.Mode),
		Recipient:          req.Recipient,
		Message:            req.Message,
		MailboxID:          req.MailboxID,
		SessionID:          req.WASessionID,
		ClientRequestID:    req.ClientRequestID,
		UserID:             userID,
		CanReadAllWhatsApp: h.allowed(c, "whatsapp.conversation.read_all"),
	})
	if err != nil {
		failQuotationDocument(c, err)
		return
	}
	corehttp.OK(c, "success", dto.QuotationSendResultResponse{
		Quotation: dto.QuotationFromDomain(res.Quotation),
		Send:      dto.QuotationSendFromDomain(res.Send),
	})
}

func (h *QuotationHandler) Summary(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	message := c.Query("message")
	if utf8.RuneCountInString(message) > 2000 {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "message max 2000 characters", http.StatusUnprocessableEntity))
		return
	}
	summary, err := h.svc.Summary(c.Request.Context(), scope, c.Param("id"), message)
	if err != nil {
		failQuotationDocument(c, err)
		return
	}
	corehttp.OK(c, "success", dto.QuotationSummaryResponse{Subject: summary.Subject, Text: summary.Text, HTML: summary.HTML})
}

func (h *QuotationHandler) Sends(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	sends, err := h.svc.ListSends(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failQuotation(c, err)
		return
	}
	out := make([]dto.QuotationSendResponse, 0, len(sends))
	for _, s := range sends {
		out = append(out, dto.QuotationSendFromDomain(s))
	}
	corehttp.OK(c, "success", out)
}
