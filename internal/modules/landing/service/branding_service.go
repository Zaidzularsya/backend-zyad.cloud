package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type brandingService struct {
	brandingRepo repository.BrandingRepository
}

func NewBrandingService(brandingRepo repository.BrandingRepository) BrandingService {
	return &brandingService{
		brandingRepo: brandingRepo,
	}
}

func (s *brandingService) UpsertDefault(ctx context.Context, scope coretenant.Scope, params repository.CreateBrandingParams) (domain.LandingBranding, error) {
	// Ensure LandingPageID is nil for default branding
	params.LandingPageID = nil
	return s.brandingRepo.Upsert(ctx, scope, params)
}

func (s *brandingService) UpsertPageOverride(ctx context.Context, scope coretenant.Scope, pageID string, params repository.CreateBrandingParams) (domain.LandingBranding, error) {
	params.LandingPageID = &pageID
	return s.brandingRepo.Upsert(ctx, scope, params)
}

func (s *brandingService) GetDefaultBranding(ctx context.Context, scope coretenant.Scope) (domain.LandingBranding, error) {
	defBranding, err := s.brandingRepo.GetDefault(ctx, scope)
	if err != nil {
		return domain.LandingBranding{}, err
	}
	return defBranding, nil
}

func (s *brandingService) GetEffectiveBranding(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingBranding, error) {
	// Fetch default branding
	defBranding, err := s.brandingRepo.GetDefault(ctx, scope)
	if err != nil {
		// If no default branding exists, initialize empty
		defBranding = domain.LandingBranding{}
	}

	// Fetch page override branding
	pageBranding, err := s.brandingRepo.GetByPage(ctx, scope, pageID)
	if err != nil {
		// If no override exists, return default
		return defBranding, nil
	}

	// Merge logic: page override takes precedence
	effective := defBranding

	if pageBranding.CompanyName != "" {
		effective.CompanyName = pageBranding.CompanyName
	}
	if pageBranding.Tagline != "" {
		effective.Tagline = pageBranding.Tagline
	}
	if pageBranding.LogoLightURL != "" {
		effective.LogoLightURL = pageBranding.LogoLightURL
	}
	if pageBranding.LogoDarkURL != "" {
		effective.LogoDarkURL = pageBranding.LogoDarkURL
	}
	if pageBranding.FaviconURL != "" {
		effective.FaviconURL = pageBranding.FaviconURL
	}
	if pageBranding.SocialImageURL != "" {
		effective.SocialImageURL = pageBranding.SocialImageURL
	}
	
	if pageBranding.Colors.Primary != "" {
		effective.Colors = pageBranding.Colors
	}
	if pageBranding.Typography.HeadingFont != "" {
		effective.Typography = pageBranding.Typography
	}
	if pageBranding.Shape.ButtonRadius != "" {
		effective.Shape = pageBranding.Shape
	}
	if pageBranding.Layout.Width != "" {
		effective.Layout = pageBranding.Layout
	}
	if pageBranding.Contact.Email != "" {
		effective.Contact = pageBranding.Contact
	}
	if len(pageBranding.SocialLinks) > 0 {
		effective.SocialLinks = pageBranding.SocialLinks
	}

	return effective, nil
}

func (s *brandingService) RemovePageOverride(ctx context.Context, scope coretenant.Scope, pageID string) error {
	return s.brandingRepo.DeleteByPage(ctx, scope, pageID)
}
