package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func validationHandlerError(err error) error {
	return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
}

type PlatformFeatureService interface {
	List(ctx context.Context, query dto.FeatureListQuery) (dto.FeatureListResponse, error)
	Create(ctx context.Context, request dto.CreateFeatureRequest) (dto.FeatureResponse, error)
	Update(ctx context.Context, id string, request dto.UpdateFeatureRequest) (dto.FeatureResponse, error)
}

type PlatformProductHandler struct {
	features PlatformFeatureService
	checker  permissionmiddleware.PermissionChecker
}

func NewPlatformProductHandler(
	features PlatformFeatureService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformProductHandler {
	return &PlatformProductHandler{
		features: features,
		checker:  checker,
	}
}

func (h *PlatformProductHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/product")
	group.Use(middleware.RequirePlatformTenant())

	group.GET("/features", permissionmiddleware.Require(h.checker, "platform.product.feature.read"), h.ListFeatures)
	group.POST("/features", permissionmiddleware.Require(h.checker, "platform.product.feature.manage"), h.CreateFeature)
	group.PATCH("/features/:id", permissionmiddleware.Require(h.checker, "platform.product.feature.manage"), h.UpdateFeature)
}

func (h *PlatformProductHandler) ListFeatures(c *gin.Context) {
	var query dto.FeatureListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "product features retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformProductHandler) CreateFeature(c *gin.Context) {
	var request dto.CreateFeatureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "product feature created successfully", result)
}

func (h *PlatformProductHandler) UpdateFeature(c *gin.Context) {
	var request dto.UpdateFeatureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.Update(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "product feature updated successfully", result)
}
