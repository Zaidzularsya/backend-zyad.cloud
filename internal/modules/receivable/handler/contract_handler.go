package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

type ContractHandler struct {
	svc      service.ContractService
	upcoming service.OverviewService // opsional: tanggal tagih berikutnya di detail
}

func NewContractHandler(svc service.ContractService) *ContractHandler {
	return &ContractHandler{svc: svc}
}

// WithUpcoming mengaktifkan field `upcoming` pada detail contract.
func (h *ContractHandler) WithUpcoming(o service.OverviewService) *ContractHandler {
	h.upcoming = o
	return h
}

func (h *ContractHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/contracts")
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "contract.read"), h.List)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "contract.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "contract.manage"), h.Patch)
	group.POST("/:id/end", permissionmiddleware.RequireOrganizationOrGlobal(p, "contract.manage"), h.End)
}

func contractID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validUUID(id) {
		failReceivable(c, receivable.ErrContractNotFound)
		return "", false
	}
	return id, true
}

func (h *ContractHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var q dto.ContractListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	list, total, err := h.svc.List(c.Request.Context(), scope, repository.ContractListFilter{
		Status: q.Status, AccountID: q.AccountID, Search: q.Search, Limit: perPage, Offset: (page - 1) * perPage})
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.ContractResponse, 0, len(list))
	for _, ct := range list {
		out = append(out, dto.ContractFromDomain(ct))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *ContractHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := contractID(c)
	if !ok {
		return
	}
	ct, err := h.svc.Get(c.Request.Context(), scope, id)
	if err != nil {
		failReceivable(c, err)
		return
	}
	detail := dto.ContractDetailResponse{ContractResponse: dto.ContractFromDomain(ct), Upcoming: []dto.UpcomingBillingResponse{}}
	if h.upcoming != nil {
		list, err := h.upcoming.ContractUpcoming(c.Request.Context(), scope, ct, 3)
		if err != nil {
			failReceivable(c, err)
			return
		}
		detail.Upcoming = dto.UpcomingFromDomain(list)
	}
	corehttp.OK(c, "success", detail)
}

func (h *ContractHandler) Patch(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := contractID(c)
	if !ok {
		return
	}
	var req dto.ContractPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	var end *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			validationFail(c, "Tanggal akhir harus berformat YYYY-MM-DD.")
			return
		}
		end = &parsed
	}
	ct, err := h.svc.SetEndDate(c.Request.Context(), scope, id, end, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ContractFromDomain(ct))
}

func (h *ContractHandler) End(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := contractID(c)
	if !ok {
		return
	}
	var req dto.ContractEndRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	var end time.Time
	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			validationFail(c, "Tanggal akhir harus berformat YYYY-MM-DD.")
			return
		}
		end = parsed
	}
	ct, err := h.svc.End(c.Request.Context(), scope, id, end, req.Reason, permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ContractFromDomain(ct))
}
