package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type seoService struct {
	pageRepo repository.PageRepository
}

func NewSeoService(pageRepo repository.PageRepository) SeoService {
	return &seoService{
		pageRepo: pageRepo,
	}
}

func (s *seoService) UpdatePageSEO(ctx context.Context, scope coretenant.Scope, pageID string, seoData map[string]any) (domain.LandingPage, error) {
	if seoData == nil {
		seoData = make(map[string]any)
	}

	// Validate Canonical URL
	if cUrl, ok := seoData["canonical_url"].(string); ok && cUrl != "" {
		if _, err := url.ParseRequestURI(cUrl); err != nil {
			return domain.LandingPage{}, errors.New("invalid canonical_url format")
		}
	}

	// Sanitize/Validate other common fields if needed
	// Example: title, description, keywords, og:image, etc.
	
	// Check if title is empty, use fallback later when resolving
	
	return s.pageRepo.Update(ctx, scope, pageID, repository.UpdatePageParams{
		SEO: seoData,
	})
}

func (s *seoService) GetEffectiveSEO(ctx context.Context, scope coretenant.Scope, pageID string) (map[string]any, error) {
	page, err := s.pageRepo.FindByID(ctx, scope, pageID)
	if err != nil {
		return nil, err
	}

	effectiveSEO := make(map[string]any)
	
	// Copy existing SEO
	if page.SEO != nil {
		for k, v := range page.SEO {
			effectiveSEO[k] = v
		}
	}

	// Generate Fallback Metadata if empty
	title, _ := effectiveSEO["title"].(string)
	if strings.TrimSpace(title) == "" {
		effectiveSEO["title"] = page.Title
	}

	desc, _ := effectiveSEO["description"].(string)
	if strings.TrimSpace(desc) == "" {
		effectiveSEO["description"] = page.Name + " - " + page.Title
	}

	// Ensure Robots defaults
	robots, _ := effectiveSEO["robots"].(string)
	if strings.TrimSpace(robots) == "" {
		if page.Visibility == domain.PageVisibilityPublic {
			effectiveSEO["robots"] = "index, follow"
		} else {
			effectiveSEO["robots"] = "noindex, nofollow"
		}
	}

	return effectiveSEO, nil
}
