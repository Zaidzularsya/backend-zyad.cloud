package handler

import (
	"context"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type CashBankService interface {
	CreateAccount(ctx context.Context, request dto.CreateCashBankAccountRequest) (dto.CashBankAccountResponse, error)
	UpdateAccount(ctx context.Context, id string, request dto.UpdateCashBankAccountRequest) (dto.CashBankAccountResponse, error)
	ListAccounts(ctx context.Context, includeInactive bool) ([]dto.CashBankAccountResponse, error)
	CreateTransaction(ctx context.Context, request dto.CreateCashTransactionRequest, actorID string) (dto.CashTransactionResponse, error)
	ListTransactions(ctx context.Context, query dto.CashTransactionListQuery) (dto.CashTransactionListResponse, error)
	CreateReconciliation(ctx context.Context, request dto.CreateBankReconciliationRequest) (dto.BankReconciliationResponse, error)
	ListReconciliations(ctx context.Context, cashBankAccountID string) ([]dto.BankReconciliationResponse, error)
	CompleteReconciliation(ctx context.Context, id string, actorID string) (dto.BankReconciliationResponse, error)
}

type PlatformCashBankHandler struct {
	cashBank CashBankService
	checker  permissionmiddleware.PermissionChecker
}

func NewPlatformCashBankHandler(cashBank CashBankService, checker permissionmiddleware.PermissionChecker) *PlatformCashBankHandler {
	return &PlatformCashBankHandler{cashBank: cashBank, checker: checker}
}

func (h *PlatformCashBankHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.cashbank.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.cashbank.manage")

	group.GET("/cash-bank-accounts", read, h.ListAccounts)
	group.POST("/cash-bank-accounts", manage, h.CreateAccount)
	group.PUT("/cash-bank-accounts/:id", manage, h.UpdateAccount)

	group.GET("/cash-transactions", read, h.ListTransactions)
	group.POST("/cash-transactions", manage, h.CreateTransaction)

	group.GET("/cash-bank-accounts/:id/reconciliations", read, h.ListReconciliations)
	group.POST("/bank-reconciliations", manage, h.CreateReconciliation)
	group.POST("/bank-reconciliations/:id/complete", manage, h.CompleteReconciliation)
}

func (h *PlatformCashBankHandler) ListAccounts(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"
	result, err := h.cashBank.ListAccounts(c.Request.Context(), includeInactive)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "cash and bank accounts retrieved successfully", result)
}

func (h *PlatformCashBankHandler) CreateAccount(c *gin.Context) {
	var request dto.CreateCashBankAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.cashBank.CreateAccount(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "cash/bank account created successfully", result)
}

func (h *PlatformCashBankHandler) UpdateAccount(c *gin.Context) {
	var request dto.UpdateCashBankAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.cashBank.UpdateAccount(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "cash/bank account updated successfully", result)
}

func (h *PlatformCashBankHandler) ListTransactions(c *gin.Context) {
	var query dto.CashTransactionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.cashBank.ListTransactions(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "cash transactions retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformCashBankHandler) CreateTransaction(c *gin.Context) {
	var request dto.CreateCashTransactionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.cashBank.CreateTransaction(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "cash transaction recorded successfully", result)
}

func (h *PlatformCashBankHandler) ListReconciliations(c *gin.Context) {
	result, err := h.cashBank.ListReconciliations(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "bank reconciliations retrieved successfully", result)
}

func (h *PlatformCashBankHandler) CreateReconciliation(c *gin.Context) {
	var request dto.CreateBankReconciliationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.cashBank.CreateReconciliation(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "bank reconciliation created successfully", result)
}

func (h *PlatformCashBankHandler) CompleteReconciliation(c *gin.Context) {
	result, err := h.cashBank.CompleteReconciliation(c.Request.Context(), c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "bank reconciliation completed successfully", result)
}
