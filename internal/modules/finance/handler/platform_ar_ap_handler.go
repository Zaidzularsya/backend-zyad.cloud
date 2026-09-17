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

type ARAPService interface {
	CreatePartner(ctx context.Context, request dto.CreateBusinessPartnerRequest) (dto.BusinessPartnerResponse, error)
	UpdatePartner(ctx context.Context, id string, request dto.UpdateBusinessPartnerRequest) (dto.BusinessPartnerResponse, error)
	ListPartners(ctx context.Context, query dto.BusinessPartnerListQuery) ([]dto.BusinessPartnerResponse, error)
	CreateTransaction(ctx context.Context, request dto.CreateARAPTransactionRequest, actorID string) (dto.ARAPTransactionResponse, error)
	ListTransactions(ctx context.Context, query dto.ARAPTransactionListQuery) (dto.ARAPTransactionListResponse, error)
	CreatePayment(ctx context.Context, request dto.CreateARAPPaymentRequest, actorID string) (dto.ARAPPaymentResponse, error)
	AgingReport(ctx context.Context, query dto.AgingReportQuery) (dto.AgingReportResponse, error)
}

type PlatformARAPHandler struct {
	arap    ARAPService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformARAPHandler(arap ARAPService, checker permissionmiddleware.PermissionChecker) *PlatformARAPHandler {
	return &PlatformARAPHandler{arap: arap, checker: checker}
}

func (h *PlatformARAPHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.arap.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.arap.manage")

	group.GET("/business-partners", read, h.ListPartners)
	group.POST("/business-partners", manage, h.CreatePartner)
	group.PUT("/business-partners/:id", manage, h.UpdatePartner)

	group.GET("/ar-ap-transactions", read, h.ListTransactions)
	group.POST("/ar-ap-transactions", manage, h.CreateTransaction)

	group.POST("/ar-ap-payments", manage, h.CreatePayment)

	group.GET("/reports/aging", read, h.AgingReport)
}

func (h *PlatformARAPHandler) ListPartners(c *gin.Context) {
	var query dto.BusinessPartnerListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.ListPartners(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "business partners retrieved successfully", result)
}

func (h *PlatformARAPHandler) CreatePartner(c *gin.Context) {
	var request dto.CreateBusinessPartnerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.CreatePartner(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "business partner created successfully", result)
}

func (h *PlatformARAPHandler) UpdatePartner(c *gin.Context) {
	var request dto.UpdateBusinessPartnerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.UpdatePartner(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "business partner updated successfully", result)
}

func (h *PlatformARAPHandler) ListTransactions(c *gin.Context) {
	var query dto.ARAPTransactionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.ListTransactions(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "AR/AP transactions retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformARAPHandler) CreateTransaction(c *gin.Context) {
	var request dto.CreateARAPTransactionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.CreateTransaction(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "AR/AP transaction created successfully", result)
}

func (h *PlatformARAPHandler) CreatePayment(c *gin.Context) {
	var request dto.CreateARAPPaymentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.CreatePayment(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "AR/AP payment recorded successfully", result)
}

func (h *PlatformARAPHandler) AgingReport(c *gin.Context) {
	var query dto.AgingReportQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.arap.AgingReport(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "aging report retrieved successfully", result)
}
