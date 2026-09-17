package handler

import (
	"context"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/finance/dto"

	"github.com/gin-gonic/gin"
)

type LedgerService interface {
	TrialBalance(ctx context.Context, query dto.TrialBalanceQuery) (dto.TrialBalanceResponse, error)
	GeneralLedger(ctx context.Context, query dto.GeneralLedgerQuery) (dto.GeneralLedgerResponse, error)
	AccountLedger(ctx context.Context, accountID string, query dto.AccountLedgerQuery) (dto.AccountLedgerResponse, error)
}

type ReportService interface {
	ProfitLoss(ctx context.Context, query dto.ProfitLossQuery) (dto.ProfitLossResponse, error)
	BalanceSheet(ctx context.Context, query dto.BalanceSheetQuery) (dto.BalanceSheetResponse, error)
}

type PlatformReportHandler struct {
	ledger  LedgerService
	reports ReportService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformReportHandler(ledger LedgerService, reports ReportService, checker permissionmiddleware.PermissionChecker) *PlatformReportHandler {
	return &PlatformReportHandler{ledger: ledger, reports: reports, checker: checker}
}

func (h *PlatformReportHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	view := permissionmiddleware.Require(h.checker, "platform.finance.reports.view")

	group.GET("/general-ledger", view, h.GeneralLedger)
	group.GET("/account-ledger/:accountId", view, h.AccountLedger)
	group.GET("/trial-balance", view, h.TrialBalance)
	group.GET("/reports/profit-loss", view, h.ProfitLoss)
	group.GET("/reports/balance-sheet", view, h.BalanceSheet)
}

func (h *PlatformReportHandler) GeneralLedger(c *gin.Context) {
	var query dto.GeneralLedgerQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.ledger.GeneralLedger(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "general ledger retrieved successfully", result)
}

func (h *PlatformReportHandler) AccountLedger(c *gin.Context) {
	var query dto.AccountLedgerQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.ledger.AccountLedger(c.Request.Context(), c.Param("accountId"), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "account ledger retrieved successfully", result)
}

func (h *PlatformReportHandler) TrialBalance(c *gin.Context) {
	var query dto.TrialBalanceQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.ledger.TrialBalance(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "trial balance retrieved successfully", result)
}

func (h *PlatformReportHandler) ProfitLoss(c *gin.Context) {
	var query dto.ProfitLossQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.reports.ProfitLoss(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "profit and loss report retrieved successfully", result)
}

func (h *PlatformReportHandler) BalanceSheet(c *gin.Context) {
	var query dto.BalanceSheetQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.reports.BalanceSheet(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "balance sheet retrieved successfully", result)
}
