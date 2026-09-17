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

type FixedAssetService interface {
	CreateCategory(ctx context.Context, request dto.CreateAssetCategoryRequest) (dto.AssetCategoryResponse, error)
	ListCategories(ctx context.Context, query dto.AssetCategoryListQuery) ([]dto.AssetCategoryResponse, error)
	CreateAsset(ctx context.Context, request dto.CreateFixedAssetRequest, actorID string) (dto.FixedAssetResponse, error)
	ListAssets(ctx context.Context, query dto.FixedAssetListQuery) (dto.FixedAssetListResponse, error)
	ListSchedules(ctx context.Context, fixedAssetID string) ([]dto.DepreciationScheduleResponse, error)
	PostDepreciation(ctx context.Context, request dto.PostDepreciationRequest, actorID string) (dto.PostDepreciationResponse, error)
}

type PlatformFixedAssetHandler struct {
	assets  FixedAssetService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformFixedAssetHandler(assets FixedAssetService, checker permissionmiddleware.PermissionChecker) *PlatformFixedAssetHandler {
	return &PlatformFixedAssetHandler{assets: assets, checker: checker}
}

func (h *PlatformFixedAssetHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/finance")
	group.Use(middleware.RequirePlatformTenant())

	read := permissionmiddleware.Require(h.checker, "platform.finance.asset.read")
	manage := permissionmiddleware.Require(h.checker, "platform.finance.asset.manage")

	group.GET("/asset-categories", read, h.ListCategories)
	group.POST("/asset-categories", manage, h.CreateCategory)

	group.GET("/fixed-assets", read, h.ListAssets)
	group.POST("/fixed-assets", manage, h.CreateAsset)
	group.GET("/fixed-assets/:id/depreciation-schedule", read, h.ListSchedules)

	group.POST("/fixed-assets/post-depreciation", manage, h.PostDepreciation)
}

func (h *PlatformFixedAssetHandler) ListCategories(c *gin.Context) {
	var query dto.AssetCategoryListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.assets.ListCategories(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "asset categories retrieved successfully", result)
}

func (h *PlatformFixedAssetHandler) CreateCategory(c *gin.Context) {
	var request dto.CreateAssetCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.assets.CreateCategory(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "asset category created successfully", result)
}

func (h *PlatformFixedAssetHandler) ListAssets(c *gin.Context) {
	var query dto.FixedAssetListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.assets.ListAssets(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "fixed assets retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformFixedAssetHandler) CreateAsset(c *gin.Context) {
	var request dto.CreateFixedAssetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.assets.CreateAsset(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "fixed asset created successfully", result)
}

func (h *PlatformFixedAssetHandler) ListSchedules(c *gin.Context) {
	result, err := h.assets.ListSchedules(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "depreciation schedule retrieved successfully", result)
}

func (h *PlatformFixedAssetHandler) PostDepreciation(c *gin.Context) {
	var request dto.PostDepreciationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.assets.PostDepreciation(c.Request.Context(), request, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "depreciation posted successfully", result)
}
