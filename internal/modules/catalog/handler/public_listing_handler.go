package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/catalog/dto"
)

type PublicListingService interface {
	Listings(ctx context.Context) (dto.PublicCatalogResponse, error)
}

// PublicListingHandler menyajikan produk publik org platform tanpa auth.
type PublicListingHandler struct{ svc PublicListingService }

func NewPublicListingHandler(svc PublicListingService) *PublicListingHandler {
	return &PublicListingHandler{svc: svc}
}

func (h *PublicListingHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/public/catalog/listings", h.List)
}

func (h *PublicListingHandler) List(c *gin.Context) {
	result, err := h.svc.Listings(c.Request.Context())
	if err != nil {
		failCatalog(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	corehttp.OK(c, "success", result)
}
