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

type TaxService interface {
	ListTypes(ctx context.Context) ([]dto.TaxTypeResponse, error)
	CreateRate(ctx context.Context, request dto.CreateTaxRateRequest) (dto.TaxRateResponse, error)
	ListRates(ctx context.Context, query dto.TaxRateListQuery) ([]dto.TaxRateResponse, error)
	CreateTransaction(ctx context.Context, request dto.CreateTaxTransactionRequest, actorID string) (dto.TaxTransactionResponse, error)
	ListTransactions(ctx context.Context, query dto.TaxTransactionListQuery) (dto.TaxTransactionListResponse, error)
	Summary(ctx context.Context, query dto.TaxSummaryQuery) (dto.TaxSummaryResponse, error)
}

type PlatformTaxHandler struct {
	tax     TaxService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformTaxHandler(tax TaxService, checker permissionmiddleware.PermissionChecker) *PlatformTaxHandler {
	return &PlatformTaxHandler{tax: tax, checker: checker}
}

func (h *PlatformTaxHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.tax.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.tax.manage")

	group.GET("/tax-types", read, h.ListTypes)

	group.GET("/tax-rates", read, h.ListRates)
	group.POST("/tax-rates", manage, h.CreateRate)

	group.GET("/tax-transactions", read, h.ListTransactions)
	group.POST("/tax-transactions", manage, h.CreateTransaction)

	group.GET("/reports/tax-summary", read, h.Summary)
}

func (h *PlatformTaxHandler) ListTypes(c *gin.Context) {
	result, err := h.tax.ListTypes(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "tax types retrieved successfully", result)
}

func (h *PlatformTaxHandler) CreateRate(c *gin.Context) {
	var request dto.CreateTaxRateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.tax.CreateRate(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "tax rate created successfully", result)
}

func (h *PlatformTaxHandler) ListRates(c *gin.Context) {
	var query dto.TaxRateListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.tax.ListRates(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "tax rates retrieved successfully", result)
}

func (h *PlatformTaxHandler) CreateTransaction(c *gin.Context) {
	var request dto.CreateTaxTransactionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.tax.CreateTransaction(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "tax transaction recorded successfully", result)
}

func (h *PlatformTaxHandler) ListTransactions(c *gin.Context) {
	var query dto.TaxTransactionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.tax.ListTransactions(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "tax transactions retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformTaxHandler) Summary(c *gin.Context) {
	var query dto.TaxSummaryQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.tax.Summary(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "tax summary retrieved successfully", result)
}
