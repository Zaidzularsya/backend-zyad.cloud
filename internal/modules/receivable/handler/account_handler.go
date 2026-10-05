package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/response"
)

type AccountHandler struct{ svc service.AccountService }

func NewAccountHandler(svc service.AccountService) *AccountHandler { return &AccountHandler{svc: svc} }

func (h *AccountHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/accounts")
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.read"), h.Get)
	group.PUT("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "invoice.update"), h.Update)
}

// input: akun lewat HTTP selalu manual; sumber (kontak CRM) hanya diisi modul lain lewat service.
func accountInput(req dto.AccountRequest) service.AccountInput {
	return service.AccountInput{Name: req.Name, CompanyName: req.CompanyName, Email: req.Email, Phone: req.Phone, Address: req.Address}
}

func (h *AccountHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var q dto.AccountListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, "Parameter tidak valid.")
		return
	}
	page, perPage := pageParams(q.Page, q.PerPage)
	accounts, total, err := h.svc.List(c.Request.Context(), scope, q.Search, perPage, (page-1)*perPage)
	if err != nil {
		failReceivable(c, err)
		return
	}
	out := make([]dto.AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, dto.AccountFromDomain(a))
	}
	response.JSON(c, http.StatusOK, "success", out, dto.BuildMeta(page, perPage, total))
}

func (h *AccountHandler) Create(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.AccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	acc, err := h.svc.Create(c.Request.Context(), scope, accountInput(req), permissionmiddleware.UserID(c))
	if err != nil {
		failReceivable(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, "success", dto.AccountFromDomain(acc), nil)
}

func (h *AccountHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := accountID(c)
	if !ok {
		return
	}
	acc, err := h.svc.Get(c.Request.Context(), scope, id)
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.AccountFromDomain(acc))
}

func (h *AccountHandler) Update(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	id, ok := accountID(c)
	if !ok {
		return
	}
	var req dto.AccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, "Isi permintaan tidak valid.")
		return
	}
	acc, err := h.svc.Update(c.Request.Context(), scope, id, accountInput(req))
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.AccountFromDomain(acc))
}
