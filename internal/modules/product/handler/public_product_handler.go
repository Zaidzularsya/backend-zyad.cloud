package handler

import (
	"context"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/product/dto"

	"github.com/gin-gonic/gin"
)

type PublicPlanService interface {
	ListPublic(ctx context.Context) (dto.PublicPlanListResponse, error)
}

type PublicProductHandler struct {
	plans PublicPlanService
}

func NewPublicProductHandler(plans PublicPlanService) *PublicProductHandler {
	return &PublicProductHandler{plans: plans}
}

func (h *PublicProductHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/public/catalog")
	group.GET("/plans", h.ListPlans)
}

func (h *PublicProductHandler) ListPlans(c *gin.Context) {
	result, err := h.plans.ListPublic(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "public product plans retrieved successfully", result.Items)
}
