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
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

// SalesOrderRefs membaca ringkasan invoice/kontrak receivable untuk respons (boleh nil = tanpa tautan).
type SalesOrderRefs struct {
	Invoices  service.InvoiceStatusReader
	Contracts service.ContractInfoReader
}

type SalesOrderHandler struct {
	svc  service.SalesOrderService
	refs SalesOrderRefs
}

func NewSalesOrderHandler(svc service.SalesOrderService, refs SalesOrderRefs) *SalesOrderHandler {
	return &SalesOrderHandler{svc: svc, refs: refs}
}

func (h *SalesOrderHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/sales-orders")
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.read"), h.List)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.manage"), h.Update)
	group.POST("/:id/cancel", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.manage"), h.Cancel)
	group.POST("/:id/confirm", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.confirm"), h.Confirm)
	group.POST("/:id/retry-billing", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.confirm"), h.RetryBilling)
	group.POST("/:id/deliveries", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.confirm"), h.ConfirmDelivery)
}

func soValidation(c *gin.Context, message string) {
	corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity))
}

func failSalesOrder(c *gin.Context, err error) {
	var incomplete *service.IncompleteError
	switch {
	case errors.As(err, &incomplete):
		response.ErrorWithData(c, http.StatusUnprocessableEntity, "SALES_ORDER_INCOMPLETE", "Data sales order belum lengkap.",
			map[string]any{"fields": incomplete.Fields})
	case errors.Is(err, repository.ErrSalesOrderNotDraft):
		corehttp.Fail(c, coreerrors.New("SALES_ORDER_NOT_DRAFT", "Sales order sudah tidak berstatus draft.", http.StatusConflict))
	case errors.Is(err, repository.ErrDeliveryNotPending):
		corehttp.Fail(c, coreerrors.New("DELIVERY_NOT_PENDING", "Ada item yang tidak sedang menunggu konfirmasi diterima.", http.StatusConflict))
	case errors.Is(err, service.ErrSalesOrderNotConfirmed):
		corehttp.Fail(c, coreerrors.New("SALES_ORDER_NOT_CONFIRMED", "Sales order belum dikonfirmasi.", http.StatusConflict))
	case errors.Is(err, service.ErrBillingNotRetryable):
		corehttp.Fail(c, coreerrors.New("BILLING_NOT_RETRYABLE", "Penagihan tidak sedang menunggu percobaan ulang.", http.StatusConflict))
	case errors.Is(err, service.ErrBillingFailed):
		corehttp.Fail(c, coreerrors.New("BILLING_FAILED", "Penagihan gagal dibuat. Coba lagi beberapa saat.", http.StatusBadGateway))
	case errors.Is(err, service.ErrBatchKeyRequired):
		soValidation(c, "batch_key wajib diisi.")
	case errors.Is(err, service.ErrNothingToDeliver):
		soValidation(c, "Pilih minimal satu item.")
	case errors.Is(err, service.ErrDeliveryDateInFuture):
		soValidation(c, "Tanggal diterima tidak boleh di masa depan.")
	case errors.Is(err, service.ErrInvalidSalesOrderFields):
		soValidation(c, err.Error())
	default:
		corehttp.Fail(c, err)
	}
}

func soScope(c *gin.Context) (coretenant.Scope, bool) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return coretenant.Scope{}, false
	}
	return scope, true
}

// respond membangun respons SO + ringkasan invoice awal/kontrak (galat baca hanya di-log).
func (h *SalesOrderHandler) respond(c *gin.Context, scope coretenant.Scope, so domain.SalesOrder) dto.SalesOrderResponse {
	ctx := c.Request.Context()
	var invoice, contract *dto.DocumentRef
	if so.InitialInvoiceID != "" && h.refs.Invoices != nil {
		if status, number, err := h.refs.Invoices.InvoiceStatus(ctx, scope, so.InitialInvoiceID); err == nil {
			invoice = &dto.DocumentRef{ID: so.InitialInvoiceID, Number: number, Status: status}
		} else {
			slog.WarnContext(ctx, "sales order: read invoice failed", "invoice_id", so.InitialInvoiceID, "error", err)
		}
	}
	if so.ContractID != "" && h.refs.Contracts != nil {
		if status, number, err := h.refs.Contracts.ContractInfo(ctx, scope, so.ContractID); err == nil {
			contract = &dto.DocumentRef{ID: so.ContractID, Number: number, Status: status}
		} else {
			slog.WarnContext(ctx, "sales order: read contract failed", "contract_id", so.ContractID, "error", err)
		}
	}
	return dto.SalesOrderFromDomain(so, invoice, contract)
}

func (h *SalesOrderHandler) List(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	var q dto.SalesOrderListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		soValidation(c, "Parameter tidak valid.")
		return
	}
	page, perPage := q.Page, q.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	orders, total, err := h.svc.List(c.Request.Context(), scope, repository.SalesOrderListFilter{
		Status: q.Status, BillingStatus: q.BillingStatus, DealID: q.DealID, Search: q.Search, Limit: perPage, Offset: (page - 1) * perPage})
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	out := make([]dto.SalesOrderResponse, 0, len(orders))
	for _, so := range orders {
		out = append(out, dto.SalesOrderFromDomain(so, nil, nil))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *SalesOrderHandler) Get(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	so, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

func (h *SalesOrderHandler) Update(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	var req dto.SalesOrderUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		soValidation(c, "Isi permintaan tidak valid.")
		return
	}
	start, err := parseDealDate(req.StartDate)
	if err != nil {
		soValidation(c, "Tanggal mulai harus berformat YYYY-MM-DD.")
		return
	}
	so, err := h.svc.UpdateDraft(c.Request.Context(), scope, c.Param("id"), repository.SalesOrderDraftFields{
		StartDate: start, BillToName: req.BillToName, BillToCompany: req.BillToCompany, BillToEmail: req.BillToEmail,
		BillToPhone: req.BillToPhone, BillToAddress: req.BillToAddress, Channels: req.Channels, PICUserID: req.PICUserID,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

func (h *SalesOrderHandler) Cancel(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	so, err := h.svc.Cancel(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

// Confirm: kegagalan penagihan tetap 200 — SO confirmed dengan billing_status "failed" dan billing_error ramah.
func (h *SalesOrderHandler) Confirm(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	so, err := h.svc.Confirm(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

func (h *SalesOrderHandler) RetryBilling(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	so, err := h.svc.RetryBilling(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

func (h *SalesOrderHandler) ConfirmDelivery(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	var req dto.SalesOrderDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		soValidation(c, "Isi permintaan tidak valid.")
		return
	}
	var at time.Time
	if req.DeliveredAt != "" {
		parsed, err := time.Parse("2006-01-02", req.DeliveredAt)
		if err != nil {
			soValidation(c, "Tanggal diterima harus berformat YYYY-MM-DD.")
			return
		}
		at = parsed
	}
	so, err := h.svc.ConfirmDelivery(c.Request.Context(), scope, c.Param("id"), service.DeliveryInput{
		ItemIDs: req.ItemIDs, DeliveredAt: at, Note: req.Note, BatchKey: req.BatchKey}, permissionmiddleware.UserID(c))
	if err != nil {
		failSalesOrder(c, err)
		return
	}
	corehttp.OK(c, "success", h.respond(c, scope, so))
}

// WonChecklistProvider: subset WonEvaluator yang dipakai route deal.
type WonChecklistProvider interface {
	Checklist(ctx context.Context, scope coretenant.Scope, dealID string) ([]service.WonChecklist, error)
	Evaluate(ctx context.Context, scope coretenant.Scope, dealID string) (bool, error)
}

// DealOrdersHandler menyajikan data order & syarat Won untuk Deal Detail.
type DealOrdersHandler struct {
	orders *SalesOrderHandler
	won    WonChecklistProvider
}

func NewDealOrdersHandler(orders *SalesOrderHandler, won WonChecklistProvider) *DealOrdersHandler {
	return &DealOrdersHandler{orders: orders, won: won}
}

func (h *DealOrdersHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/deals")
	group.GET("/:id/won-checklist", permissionmiddleware.RequireOrganizationOrGlobal(p, "deal.read"), h.WonChecklist)
	group.GET("/:id/sales-orders", permissionmiddleware.RequireOrganizationOrGlobal(p, "sales_order.read"), h.SalesOrders)
}

// WonChecklist menjalankan Evaluate lebih dulu: memulihkan Won bila listener receivable pernah gagal.
func (h *DealOrdersHandler) WonChecklist(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if _, err := h.won.Evaluate(ctx, scope, c.Param("id")); err != nil {
		slog.WarnContext(ctx, "won checklist: evaluate failed", "deal_id", c.Param("id"), "error", err)
	}
	lists, err := h.won.Checklist(ctx, scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.WonChecklistsFromService(lists))
}

func (h *DealOrdersHandler) SalesOrders(c *gin.Context) {
	scope, ok := soScope(c)
	if !ok {
		return
	}
	orders, err := h.orders.svc.ListByDeal(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	out := make([]dto.SalesOrderResponse, 0, len(orders))
	for _, so := range orders {
		out = append(out, h.orders.respond(c, scope, so))
	}
	corehttp.OK(c, "success", out)
}
