package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminBrandingHandler struct {
	brandingSvc service.BrandingService
}

func NewAdminBrandingHandler(brandingSvc service.BrandingService) *AdminBrandingHandler {
	return &AdminBrandingHandler{
		brandingSvc: brandingSvc,
	}
}

func (h *AdminBrandingHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	// Organization branding
	router.GET("/admin/landing/branding", permissionmiddleware.Require(checker, "landing.branding.read"), h.GetDefaultBranding)
	router.PATCH("/admin/landing/branding", permissionmiddleware.Require(checker, "landing.branding.update"), h.UpdateDefaultBranding)

	// Theme (subset of branding)
	router.GET("/admin/landing/theme", permissionmiddleware.Require(checker, "landing.branding.read"), h.GetTheme)
	router.PATCH("/admin/landing/theme", permissionmiddleware.Require(checker, "landing.theme.manage"), h.UpdateTheme)

	// Page override branding
	pageGroup := router.Group("/admin/landing-pages/:id/branding")
	pageGroup.GET("", permissionmiddleware.Require(checker, "landing.branding.read"), h.GetPageBranding)
	pageGroup.PATCH("", permissionmiddleware.Require(checker, "landing.branding.update"), h.UpdatePageBranding)
	pageGroup.DELETE("", permissionmiddleware.Require(checker, "landing.branding.update"), h.DeletePageBranding)
}

func (h *AdminBrandingHandler) GetDefaultBranding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	branding, err := h.brandingSvc.GetDefaultBranding(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Default branding retrieved successfully", mapBrandingResponse(branding))
}

func (h *AdminBrandingHandler) UpdateDefaultBranding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.ThemeBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := h.mapThemeBrandingRequestToParams(req, permissionmiddleware.UserID(c))
	branding, err := h.brandingSvc.UpsertDefault(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Default branding updated successfully", mapBrandingResponse(branding))
}

func (h *AdminBrandingHandler) GetTheme(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	branding, err := h.brandingSvc.GetDefaultBranding(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	themeData := map[string]any{
		"colors":     branding.Colors,
		"typography": branding.Typography,
		"shape":      branding.Shape,
		"layout":     branding.Layout,
	}

	corehttp.OK(c, "Theme retrieved successfully", themeData)
}

func (h *AdminBrandingHandler) UpdateTheme(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.ThemeBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := h.mapThemeBrandingRequestToParams(req, permissionmiddleware.UserID(c))
	branding, err := h.brandingSvc.UpsertDefault(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Theme updated successfully", mapBrandingResponse(branding))
}

func (h *AdminBrandingHandler) GetPageBranding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	branding, err := h.brandingSvc.GetEffectiveBranding(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page branding retrieved successfully", mapBrandingResponse(branding))
}

func (h *AdminBrandingHandler) UpdatePageBranding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	var req dto.ThemeBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := h.mapThemeBrandingRequestToParams(req, permissionmiddleware.UserID(c))
	branding, err := h.brandingSvc.UpsertPageOverride(c.Request.Context(), scope, pageID, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page branding override updated successfully", mapBrandingResponse(branding))
}

func mapBrandingResponse(branding domain.LandingBranding) dto.BrandingResponse {
	socialLinks := make([]map[string]any, 0, len(branding.SocialLinks))
	for _, link := range branding.SocialLinks {
		socialLinks = append(socialLinks, map[string]any{
			"platform": link.Platform,
			"url":      link.URL,
		})
	}

	return dto.BrandingResponse{
		CompanyName:    branding.CompanyName,
		Tagline:        branding.Tagline,
		LogoLightURL:   branding.LogoLightURL,
		LogoDarkURL:    branding.LogoDarkURL,
		FaviconURL:     branding.FaviconURL,
		SocialImageURL: branding.SocialImageURL,
		Colors: map[string]any{
			"primary":    branding.Colors.Primary,
			"secondary":  branding.Colors.Secondary,
			"accent":     branding.Colors.Accent,
			"background": branding.Colors.Background,
			"surface":    branding.Colors.Surface,
			"text":       branding.Colors.Text,
			"muted":      branding.Colors.Muted,
		},
		Typography: map[string]any{
			"heading_font": branding.Typography.HeadingFont,
			"body_font":    branding.Typography.BodyFont,
		},
		Shape: map[string]any{
			"button_radius": branding.Shape.ButtonRadius,
			"card_radius":   branding.Shape.CardRadius,
		},
		Layout: map[string]any{
			"width":            branding.Layout.Width,
			"spacing":          branding.Layout.Spacing,
			"background_style": branding.Layout.BackgroundStyle,
			"color_mode":       branding.Layout.ColorMode,
			"header_style":     branding.Layout.HeaderStyle,
			"footer_style":     branding.Layout.FooterStyle,
		},
		Contact: map[string]any{
			"email":   branding.Contact.Email,
			"phone":   branding.Contact.Phone,
			"address": branding.Contact.Address,
		},
		SocialLinks: socialLinks,
		CreatedAt:   branding.CreatedAt,
		UpdatedAt:   branding.UpdatedAt,
	}
}

func (h *AdminBrandingHandler) DeletePageBranding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	if err := h.brandingSvc.RemovePageOverride(c.Request.Context(), scope, pageID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page branding override removed successfully", nil)
}

func (h *AdminBrandingHandler) mapThemeBrandingRequestToParams(req dto.ThemeBrandingRequest, _ string) repository.CreateBrandingParams {
	var params repository.CreateBrandingParams

	params.CompanyName = req.CompanyName
	params.Tagline = req.Tagline
	params.LogoLightURL = req.LogoLightURL
	params.LogoDarkURL = req.LogoDarkURL
	params.FaviconURL = req.FaviconURL
	params.SocialImageURL = req.SocialImageURL

	if req.Colors != nil {
		b, _ := json.Marshal(req.Colors)
		var colors domain.BrandingColors
		json.Unmarshal(b, &colors)
		params.Colors = &colors
	}
	if req.Typography != nil {
		b, _ := json.Marshal(req.Typography)
		var typography domain.BrandingTypography
		json.Unmarshal(b, &typography)
		params.Typography = &typography
	}
	if req.Shape != nil {
		b, _ := json.Marshal(req.Shape)
		var shape domain.BrandingShape
		json.Unmarshal(b, &shape)
		params.Shape = &shape
	}
	if req.Layout != nil {
		b, _ := json.Marshal(req.Layout)
		var layout domain.BrandingLayout
		json.Unmarshal(b, &layout)
		params.Layout = &layout
	}
	if req.Contact != nil {
		b, _ := json.Marshal(req.Contact)
		var contact domain.BrandingContact
		json.Unmarshal(b, &contact)
		params.Contact = &contact
	}
	if req.SocialLinks != nil {
		b, _ := json.Marshal(req.SocialLinks)
		var links []domain.BrandingSocialLink
		json.Unmarshal(b, &links)
		params.SocialLinks = links
	}
	return params
}
