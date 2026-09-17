package handler

import (
	"context"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/finance/dto"

	"github.com/gin-gonic/gin"
)

type AccountService interface {
	ListAccountTypes(ctx context.Context) ([]dto.AccountTypeResponse, error)
	ListAccountCategories(ctx context.Context) ([]dto.AccountCategoryResponse, error)
	Create(ctx context.Context, request dto.CreateAccountRequest) (dto.AccountResponse, error)
	Update(ctx context.Context, id string, request dto.UpdateAccountRequest) (dto.AccountResponse, error)
	Get(ctx context.Context, id string) (dto.AccountResponse, error)
	List(ctx context.Context, query dto.AccountListQuery) ([]dto.AccountResponse, error)
}

type FiscalService interface {
	CreateFiscalYear(ctx context.Context, request dto.CreateFiscalYearRequest) (dto.FiscalYearResponse, error)
	ListFiscalYears(ctx context.Context) ([]dto.FiscalYearResponse, error)
	ClosePeriod(ctx context.Context, periodID string, actorID string) (dto.FiscalPeriodResponse, error)
	ReopenPeriod(ctx context.Context, periodID string, actorID string) (dto.FiscalPeriodResponse, error)
	CloseFiscalYear(ctx context.Context, fiscalYearID string, actorID string) (dto.FiscalYearResponse, error)
	ReopenFiscalYear(ctx context.Context, fiscalYearID string, actorID string) (dto.FiscalYearResponse, error)
}

type PlatformCoAHandler struct {
	accounts AccountService
	fiscal   FiscalService
	checker  permissionmiddleware.PermissionChecker
}

func NewPlatformCoAHandler(accounts AccountService, fiscal FiscalService, checker permissionmiddleware.PermissionChecker) *PlatformCoAHandler {
	return &PlatformCoAHandler{accounts: accounts, fiscal: fiscal, checker: checker}
}

func (h *PlatformCoAHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.coa.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.coa.manage")

	group.GET("/account-types", read, h.ListAccountTypes)
	group.GET("/account-categories", read, h.ListAccountCategories)
	group.GET("/accounts", read, h.ListAccounts)
	group.GET("/accounts/:id", read, h.GetAccount)
	group.POST("/accounts", manage, h.CreateAccount)
	group.PUT("/accounts/:id", manage, h.UpdateAccount)

	group.GET("/fiscal-years", read, h.ListFiscalYears)
	group.POST("/fiscal-years", manage, h.CreateFiscalYear)
	group.POST("/fiscal-years/:id/close", manage, h.CloseFiscalYear)
	group.POST("/fiscal-years/:id/reopen", manage, h.ReopenFiscalYear)
	group.POST("/fiscal-periods/:id/close", manage, h.ClosePeriod)
	group.POST("/fiscal-periods/:id/reopen", manage, h.ReopenPeriod)
}

func (h *PlatformCoAHandler) ListAccountTypes(c *gin.Context) {
	result, err := h.accounts.ListAccountTypes(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "finance account types retrieved successfully", result)
}

func (h *PlatformCoAHandler) ListAccountCategories(c *gin.Context) {
	result, err := h.accounts.ListAccountCategories(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "finance account categories retrieved successfully", result)
}

func (h *PlatformCoAHandler) ListAccounts(c *gin.Context) {
	var query dto.AccountListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.accounts.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "chart of accounts retrieved successfully", result)
}

func (h *PlatformCoAHandler) GetAccount(c *gin.Context) {
	result, err := h.accounts.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "account retrieved successfully", result)
}

func (h *PlatformCoAHandler) CreateAccount(c *gin.Context) {
	var request dto.CreateAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.accounts.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "account created successfully", result)
}

func (h *PlatformCoAHandler) UpdateAccount(c *gin.Context) {
	var request dto.UpdateAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.accounts.Update(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "account updated successfully", result)
}

func (h *PlatformCoAHandler) ListFiscalYears(c *gin.Context) {
	result, err := h.fiscal.ListFiscalYears(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "fiscal years retrieved successfully", result)
}

func (h *PlatformCoAHandler) CreateFiscalYear(c *gin.Context) {
	var request dto.CreateFiscalYearRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.fiscal.CreateFiscalYear(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "fiscal year created successfully", result)
}

func (h *PlatformCoAHandler) CloseFiscalYear(c *gin.Context) {
	result, err := h.fiscal.CloseFiscalYear(c.Request.Context(), c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "fiscal year closed successfully", result)
}

func (h *PlatformCoAHandler) ReopenFiscalYear(c *gin.Context) {
	result, err := h.fiscal.ReopenFiscalYear(c.Request.Context(), c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "fiscal year reopened successfully", result)
}

func (h *PlatformCoAHandler) ClosePeriod(c *gin.Context) {
	result, err := h.fiscal.ClosePeriod(c.Request.Context(), c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "fiscal period closed successfully", result)
}

func (h *PlatformCoAHandler) ReopenPeriod(c *gin.Context) {
	result, err := h.fiscal.ReopenPeriod(c.Request.Context(), c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "fiscal period reopened successfully", result)
}
