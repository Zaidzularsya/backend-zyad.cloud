package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

const (
	permRecordArrears = "invoice.record_arrears"
	permIsolirFlag    = "invoice.isolir_flag"
)

// ArrearsHandler: recording old arrears (tunggakan) as back-dated invoices, and the owner-only
// isolation flag on those invoices.
type ArrearsHandler struct {
	arrears service.ArrearsService
	perm    permissionmiddleware.CombinedPermissionChecker
}

func NewArrearsHandler(arrears service.ArrearsService) *ArrearsHandler {
	return &ArrearsHandler{arrears: arrears}
}

func (h *ArrearsHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	router.GET("/contracts/:id/arrears/periods", permissionmiddleware.RequireOrganizationOrGlobal(p, permRecordArrears), h.Periods)
	// Static /invoices/arrears sits beside /invoices/:id (gin routes static segments first).
	router.POST("/invoices/arrears", permissionmiddleware.RequireOrganizationOrGlobal(p, permRecordArrears), h.Record)
	router.POST("/invoices/arrears/:recordingId/resend", permissionmiddleware.RequireOrganizationOrGlobal(p, permRecordArrears), h.Resend)
	router.PUT("/invoices/:id/isolir-flag", permissionmiddleware.RequireOrganizationOrGlobal(p, permIsolirFlag), h.SetIsolirFlag)
}

func (h *ArrearsHandler) Periods(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failArrears(c, service.ErrArrearsContractMissing)
		return
	}
	rows, hasTrigger, err := h.arrears.Preview(c.Request.Context(), scope, id)
	if err != nil {
		failArrears(c, err)
		return
	}
	out := dto.ArrearsPeriodsResponse{Periods: make([]dto.ArrearsPeriodResponse, len(rows)), HasTriggerProduct: hasTrigger}
	for i, r := range rows {
		out.Periods[i] = dto.ArrearsPeriodResponse{
			Index: r.Period.Index, Start: r.Period.Start.Format(time.DateOnly), End: r.Period.End.Format(time.DateOnly),
			DefaultAmount: r.Period.DefaultAmount, Conflict: r.Conflict,
		}
	}
	corehttp.OK(c, "success", out)
}

func (h *ArrearsHandler) Record(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	key := trim(c.GetHeader("Idempotency-Key"))
	if key == "" || utf8.RuneCountInString(key) > 100 {
		validationFail(c, "Header Idempotency-Key wajib diisi (maksimal 100 karakter).")
		return
	}
	var req dto.ArrearsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Data pencatatan tunggakan tidak valid.")
		return
	}
	if !validUUID(req.AccountID) || !validUUID(req.ContractID) {
		validationFail(c, "Pelanggan atau kontrak tidak valid.")
		return
	}
	in := service.ArrearsInput{AccountID: req.AccountID, ContractID: req.ContractID, Reason: req.Reason, IdempotencyKey: key}
	flagged := false
	for i, r := range req.Rows {
		start, ok1 := parseDate(r.PeriodStart)
		due, ok2 := parseDate(r.DueDate)
		if !ok1 || !ok2 || start == nil || due == nil {
			validationFail(c, "Baris "+strconv.Itoa(i+1)+": tanggal periode dan jatuh tempo wajib dengan format YYYY-MM-DD.")
			return
		}
		in.Rows = append(in.Rows, service.ArrearsRowInput{PeriodStart: *start, Amount: r.Amount, DueDate: *due, IsolirFlag: r.IsolirFlag})
		flagged = flagged || r.IsolirFlag
	}
	// Setting the flag while recording is the same decision as changing it later: owner only.
	if flagged && !allowed(h.perm, c, permIsolirFlag) {
		forbidden(c)
		return
	}
	res, err := h.arrears.Record(c.Request.Context(), scope, in, permissionmiddleware.UserID(c))
	if err != nil {
		failArrears(c, err)
		return
	}
	out := dto.ArrearsResultResponse{Recording: dto.ArrearsRecordingFromDomain(res.Recording),
		Invoices: make([]dto.InvoiceResponse, len(res.Invoices)), NotifyStatus: res.Recording.NotifyStatus}
	for i, inv := range res.Invoices {
		out.Invoices[i] = dto.InvoiceFromDomain(inv, nil)
	}
	corehttp.Created(c, "success", out)
}

func (h *ArrearsHandler) Resend(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id := c.Param("recordingId")
	if !validUUID(id) {
		failArrears(c, receivable.ErrInvoiceNotFound)
		return
	}
	rec, err := h.arrears.Resend(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failArrears(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ArrearsRecordingFromDomain(rec))
}

func (h *ArrearsHandler) SetIsolirFlag(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	var req dto.IsolirFlagRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Value == nil {
		validationFail(c, "Nilai penanda wajib (true/false).")
		return
	}
	inv, err := h.arrears.SetIsolirFlag(c.Request.Context(), scope, id, *req.Value, permissionmiddleware.UserID(c))
	if err != nil {
		failArrears(c, err)
		return
	}
	corehttp.OK(c, "success", dto.InvoiceFromDomain(inv, nil))
}

func failArrears(c *gin.Context, err error) {
	var conflict *service.ArrearsConflictError
	switch {
	case errors.As(err, &conflict):
		list := make([]dto.ArrearsConflictResponse, len(conflict.Conflicts))
		for i, x := range conflict.Conflicts {
			list[i] = dto.ArrearsConflictResponse{Row: x.Row, PeriodStart: x.PeriodStart, Reason: x.Reason}
		}
		response.ErrorWithData(c, http.StatusConflict, "ARREARS_CONFLICT",
			"Ada periode yang sudah punya invoice. Tidak ada tunggakan yang dicatat.", gin.H{"conflicts": list})
	case errors.Is(err, service.ErrArrearsContractMissing):
		corehttp.Fail(c, coreerrors.New("ARREARS_CONTRACT_MISSING", service.ErrArrearsContractMissing.Error(), http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrArrearsInvalid):
		validationFail(c, err.Error())
	default:
		failReceivable(c, err)
	}
}
