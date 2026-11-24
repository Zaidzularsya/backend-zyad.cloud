package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

// PayoutAccountHandler: rekening pencairan. Owner (/app/settlement) mengajukan/membatalkan rekening
// org sendiri (org selalu dari konteks tenant); super admin (/platform/settlements, settlement.manage)
// memverifikasi, menolak, dan menetapkan rekening atas nama org.
type PayoutAccountHandler struct{ svc service.PayoutAccountService }

func NewPayoutAccountHandler(svc service.PayoutAccountService) *PayoutAccountHandler {
	return &PayoutAccountHandler{svc: svc}
}

func (h *PayoutAccountHandler) RegisterPlatformRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	group := router.Group("/platform/settlements")
	group.Use(middleware.RequireActiveTenant(), middleware.RequirePlatformTenant(), permissionmiddleware.Require(checker, "settlement.manage"))
	// Super admin tidak boleh memanggil /app/settlement/banks (khusus customer), jadi daftar yang sama disediakan di sini.
	group.GET("/banks", h.Banks)
	group.GET("/payout-accounts", h.ListPending)
	group.POST("/payout-accounts/:id/verify", h.Verify)
	group.POST("/payout-accounts/:id/reject", h.Reject)
	group.GET("/:orgId/payout-accounts", h.PlatformHistory)
	group.POST("/:orgId/payout-accounts", h.Assign)
}

func (h *PayoutAccountHandler) RegisterOwnRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/app/settlement")
	group.Use(middleware.RequireActiveTenant(), middleware.RequireCustomerTenant(), permissionmiddleware.RequireOrganizationOrGlobal(checker, "settlement.read"))
	manage := permissionmiddleware.RequireOrganizationOrGlobal(checker, "payout_account.manage")
	group.GET("/payout-accounts", h.OwnOverview)
	group.POST("/payout-accounts", manage, h.Submit)
	group.POST("/payout-accounts/:id/cancel", manage, h.Cancel)
	group.GET("/banks", h.Banks)
}

func payoutAudit(c *gin.Context) repository.PayoutAudit {
	return repository.PayoutAudit{ActorUserID: permissionmiddleware.UserID(c), IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func payoutAccountID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validUUID(id) {
		failPayout(c, repository.ErrPayoutAccountNotFound)
		return "", false
	}
	return id, true
}

func (h *PayoutAccountHandler) Banks(c *gin.Context) { corehttp.OK(c, "success", dto.BanksResponse()) }

func (h *PayoutAccountHandler) OwnOverview(c *gin.Context) {
	orgID, ok := ownOrganizationID(c)
	if !ok {
		return
	}
	ov, err := h.svc.Overview(c.Request.Context(), orgID)
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PayoutOverviewFromService(ov))
}

func (h *PayoutAccountHandler) Submit(c *gin.Context) {
	orgID, ok := ownOrganizationID(c)
	if !ok {
		return
	}
	var req dto.PayoutAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Permintaan tidak valid.")
		return
	}
	acc, err := h.svc.Submit(c.Request.Context(), orgID, service.PayoutAccountInput{BankCode: req.BankCode, AccountNumber: req.AccountNumber, AccountHolderName: req.AccountHolderName}, payoutAudit(c))
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.Created(c, "created", dto.PayoutAccountFromRepo(acc))
}

func (h *PayoutAccountHandler) Cancel(c *gin.Context) {
	orgID, ok := ownOrganizationID(c)
	if !ok {
		return
	}
	id, ok := payoutAccountID(c)
	if !ok {
		return
	}
	acc, err := h.svc.Cancel(c.Request.Context(), orgID, id, payoutAudit(c))
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PayoutAccountFromRepo(acc))
}

func (h *PayoutAccountHandler) ListPending(c *gin.Context) {
	var q dto.PayoutPendingQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	// Hanya antrean pending yang didukung; status lain ditolak agar kontrak tidak ambigu.
	if q.Status != "" && q.Status != "pending" {
		validationFail(c, "Status hanya mendukung nilai pending.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	rows, total, err := h.svc.ListPending(c.Request.Context(), perPage, (page-1)*perPage)
	if err != nil {
		failPayout(c, err)
		return
	}
	out := make([]dto.PendingPayoutAccountResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.PendingPayoutAccountResponse{PayoutAccountResponse: dto.PayoutAccountFromRepo(r.PayoutAccount), OrganizationName: r.OrganizationName})
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *PayoutAccountHandler) Verify(c *gin.Context) {
	id, ok := payoutAccountID(c)
	if !ok {
		return
	}
	var req dto.PayoutReviewRequest
	if !bindOptionalJSON(c, &req) {
		return
	}
	acc, err := h.svc.Verify(c.Request.Context(), id, req.Note, payoutAudit(c))
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PayoutAccountFromRepo(acc))
}

func (h *PayoutAccountHandler) Reject(c *gin.Context) {
	id, ok := payoutAccountID(c)
	if !ok {
		return
	}
	var req dto.PayoutReviewRequest
	if !bindOptionalJSON(c, &req) {
		return
	}
	acc, err := h.svc.Reject(c.Request.Context(), id, req.Note, payoutAudit(c))
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PayoutAccountFromRepo(acc))
}

func (h *PayoutAccountHandler) PlatformHistory(c *gin.Context) {
	orgID, ok := beneficiaryID(c)
	if !ok {
		return
	}
	hist, err := h.svc.History(c.Request.Context(), orgID)
	if err != nil {
		failPayout(c, err)
		return
	}
	out := make([]dto.PayoutAccountResponse, 0, len(hist))
	for _, a := range hist {
		out = append(out, dto.PayoutAccountFromRepo(a))
	}
	corehttp.OK(c, "success", out)
}

func (h *PayoutAccountHandler) Assign(c *gin.Context) {
	orgID, ok := beneficiaryID(c)
	if !ok {
		return
	}
	var req dto.PayoutAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Permintaan tidak valid.")
		return
	}
	acc, err := h.svc.Assign(c.Request.Context(), orgID, service.PayoutAccountInput{BankCode: req.BankCode, AccountNumber: req.AccountNumber, AccountHolderName: req.AccountHolderName}, req.Note, payoutAudit(c))
	if err != nil {
		failPayout(c, err)
		return
	}
	corehttp.Created(c, "created", dto.PayoutAccountFromRepo(acc))
}

// bindOptionalJSON: body kosong dianggap {} (verify boleh tanpa catatan); JSON rusak = 422.
func bindOptionalJSON(c *gin.Context, dst any) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(dst); err != nil {
		validationFail(c, "Permintaan tidak valid.")
		return false
	}
	return true
}

func failPayout(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrPayoutAccountPendingExists):
		corehttp.Fail(c, coreerrors.New("PAYOUT_ACCOUNT_PENDING_EXISTS", "Masih ada pengajuan rekening yang menunggu verifikasi.", http.StatusConflict))
	case errors.Is(err, repository.ErrPayoutAccountNotFound):
		corehttp.Fail(c, coreerrors.New("PAYOUT_ACCOUNT_NOT_FOUND", "Rekening pencairan tidak ditemukan.", http.StatusNotFound))
	case errors.Is(err, repository.ErrPayoutAccountInvalidState):
		corehttp.Fail(c, coreerrors.New("PAYOUT_ACCOUNT_INVALID_STATE", "Status rekening tidak mengizinkan tindakan ini.", http.StatusConflict))
	case errors.Is(err, service.ErrPayoutNoteRequired):
		validationFail(c, "Catatan wajib diisi.")
	case errors.Is(err, service.ErrInvalidPayoutAccount):
		validationFail(c, err.Error())
	default:
		failSettlement(c, err)
	}
}
