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
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

// SettlementHandler: ledger penyelesaian. Dua permukaan:
//   - /platform/settlements*: hanya org platform + izin settlement.manage (super admin).
//   - /app/settlement*: hanya-baca untuk org customer pemanggil; organization_id SELALU dari
//     konteks tenant, tidak pernah dari path/query/body.
type SettlementHandler struct{ svc service.SettlementService }

func NewSettlementHandler(svc service.SettlementService) *SettlementHandler {
	return &SettlementHandler{svc: svc}
}

func (h *SettlementHandler) RegisterPlatformRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	group := router.Group("/platform/settlements")
	group.Use(middleware.RequireActiveTenant(), middleware.RequirePlatformTenant(), permissionmiddleware.Require(checker, "settlement.manage"))
	group.GET("", h.ListBalances)
	group.GET("/:orgId/entries", h.PlatformEntries)
	group.POST("/:orgId/entries", h.Record)
}

func (h *SettlementHandler) RegisterOwnRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/app/settlement")
	group.Use(middleware.RequireActiveTenant(), middleware.RequireCustomerTenant(), permissionmiddleware.RequireOrganizationOrGlobal(checker, "settlement.read"))
	group.GET("/balance", h.OwnBalance)
	group.GET("/entries", h.OwnEntries)
}

func (h *SettlementHandler) ListBalances(c *gin.Context) {
	var q dto.SettlementListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	rows, total, err := h.svc.ListBalances(c.Request.Context(), service.SettlementBalanceFilter{Search: trim(q.Search), Limit: perPage, Offset: (page - 1) * perPage})
	if err != nil {
		failSettlement(c, err)
		return
	}
	out := make([]dto.SettlementOrgBalanceResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.SettlementOrgBalanceFromService(r))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *SettlementHandler) PlatformEntries(c *gin.Context) {
	orgID, ok := beneficiaryID(c)
	if !ok {
		return
	}
	h.entries(c, orgID)
}

func (h *SettlementHandler) Record(c *gin.Context) {
	orgID, ok := beneficiaryID(c)
	if !ok {
		return
	}
	var req dto.SettlementEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Permintaan tidak valid.")
		return
	}
	in := service.SettlementEntryInput{Kind: req.Kind, Amount: req.Amount, Note: req.Note}
	if req.OccurredAt != nil {
		in.OccurredAt = *req.OccurredAt
	}
	e, err := h.svc.Record(c.Request.Context(), orgID, in, permissionmiddleware.UserID(c))
	if err != nil {
		failSettlement(c, err)
		return
	}
	corehttp.Created(c, "created", dto.SettlementEntryFromRepo(e))
}

func (h *SettlementHandler) OwnBalance(c *gin.Context) {
	orgID, ok := ownOrganizationID(c)
	if !ok {
		return
	}
	b, err := h.svc.Balance(c.Request.Context(), orgID)
	if err != nil {
		failSettlement(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SettlementBalanceFromService(b))
}

func (h *SettlementHandler) OwnEntries(c *gin.Context) {
	orgID, ok := ownOrganizationID(c)
	if !ok {
		return
	}
	h.entries(c, orgID)
}

func (h *SettlementHandler) entries(c *gin.Context, orgID string) {
	var q dto.SettlementListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	rows, total, err := h.svc.ListEntries(c.Request.Context(), orgID, perPage, (page-1)*perPage)
	if err != nil {
		failSettlement(c, err)
		return
	}
	out := make([]dto.SettlementEntryResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.SettlementEntryFromRepo(r))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

// ownOrganizationID: satu-satunya sumber org untuk /app/settlement adalah konteks tenant terverifikasi.
func ownOrganizationID(c *gin.Context) (string, bool) {
	tc, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return "", false
	}
	return tc.OrganizationID(), true
}

func beneficiaryID(c *gin.Context) (string, bool) {
	id := c.Param("orgId")
	if !validUUID(id) {
		failSettlement(c, service.ErrSettlementBeneficiaryNotFound)
		return "", false
	}
	return id, true
}

func failSettlement(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSettlementExceedsBalance):
		corehttp.Fail(c, coreerrors.New("SETTLEMENT_EXCEEDS_BALANCE", "Jumlah setoran melebihi sisa saldo organisasi.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrAdjustmentBelowZero):
		corehttp.Fail(c, coreerrors.New("ADJUSTMENT_BELOW_ZERO", "Penyesuaian tidak boleh membuat sisa saldo di bawah nol.", http.StatusUnprocessableEntity))
	case errors.Is(err, service.ErrAdjustmentNoteRequired):
		validationFail(c, "Catatan wajib diisi untuk penyesuaian.")
	case errors.Is(err, service.ErrInvalidSettlementEntry):
		validationFail(c, "Jenis harus settlement/adjustment, jumlah desimal maksimal 2 digit (tidak nol; negatif hanya untuk adjustment), catatan maksimal 500 karakter.")
	case errors.Is(err, service.ErrSettlementBeneficiaryNotFound):
		corehttp.Fail(c, coreerrors.New("ORGANIZATION_NOT_FOUND", "Organisasi tidak ditemukan.", http.StatusNotFound))
	case errors.Is(err, service.ErrSettlementBeneficiaryInvalid):
		corehttp.Fail(c, coreerrors.New("BENEFICIARY_NOT_CUSTOMER", "Hanya organisasi customer yang memiliki saldo penyelesaian.", http.StatusUnprocessableEntity))
	default:
		corehttp.Fail(c, err)
	}
}
