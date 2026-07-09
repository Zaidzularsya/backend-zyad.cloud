package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateBrandingParams struct {
	LandingPageID  *string
	CompanyName    *string
	Tagline        *string
	LogoLightURL   *string
	LogoDarkURL    *string
	FaviconURL     *string
	SocialImageURL *string
	Colors         *domain.BrandingColors
	Typography     *domain.BrandingTypography
	Shape          *domain.BrandingShape
	Layout         *domain.BrandingLayout
	Contact        *domain.BrandingContact
	SocialLinks    []domain.BrandingSocialLink
}

type BrandingRepository interface {
	// Branding Operations
	Upsert(context.Context, coretenant.Scope, CreateBrandingParams) (domain.LandingBranding, error)
	GetDefault(context.Context, coretenant.Scope) (domain.LandingBranding, error)
	GetByPage(context.Context, coretenant.Scope, string) (domain.LandingBranding, error)
	DeleteByPage(context.Context, coretenant.Scope, string) error
}
