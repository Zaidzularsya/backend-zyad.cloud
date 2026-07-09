package service

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

const pricingSourcePlatformCatalog = "platform_catalog"

func errPricingSourceNotAllowed() error {
	return coreerrors.New(
		"PRICING_SOURCE_NOT_ALLOWED",
		"only the platform organization may use source=platform_catalog for a pricing section",
		http.StatusBadRequest,
	)
}

func validatePricingSource(sectionType domain.SectionType, content map[string]any, organizationType coretenant.OrganizationType) error {
	if sectionType != domain.SectionTypePricing || content == nil {
		return nil
	}
	source, _ := content["source"].(string)
	if source != pricingSourcePlatformCatalog {
		return nil
	}
	if organizationType != coretenant.OrganizationTypePlatform {
		return errPricingSourceNotAllowed()
	}
	return nil
}

type sectionService struct {
	sectionRepo repository.SectionRepository
	quotaGuard  LandingSectionQuotaGuard
}

type LandingSectionQuotaGuard interface {
	RequireQuotaValue(
		context.Context,
		string,
		string,
		string,
		int64,
		int64,
	) error
}

type SectionServiceOption func(*sectionService)

func WithLandingSectionQuotaGuard(guard LandingSectionQuotaGuard) SectionServiceOption {
	return func(service *sectionService) {
		service.quotaGuard = guard
	}
}

func NewSectionService(
	sectionRepo repository.SectionRepository,
	options ...SectionServiceOption,
) SectionService {
	service := &sectionService{
		sectionRepo: sectionRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *sectionService) Create(ctx context.Context, scope coretenant.Scope, organizationType coretenant.OrganizationType, params repository.CreateSectionParams) (domain.LandingSection, error) {
	if err := validatePricingSource(params.Type, params.Content, organizationType); err != nil {
		return domain.LandingSection{}, err
	}
	if err := s.requireCreateQuota(ctx, scope, params.LandingPageID); err != nil {
		return domain.LandingSection{}, err
	}
	// Sanitize content
	params.Content = s.sanitizeMap(params.Content)
	return s.sectionRepo.Create(ctx, scope, params)
}

func (s *sectionService) requireCreateQuota(
	ctx context.Context,
	scope coretenant.Scope,
	pageID string,
) error {
	if s.quotaGuard == nil {
		return nil
	}
	sections, err := s.sectionRepo.ListByPage(ctx, scope, pageID)
	if err != nil {
		return err
	}
	return s.quotaGuard.RequireQuotaValue(
		ctx,
		scope.OrganizationID(),
		domain.FeatureLandingMaxSections,
		"limit",
		int64(len(sections)),
		1,
	)
}

func (s *sectionService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSection, error) {
	return s.sectionRepo.FindByID(ctx, scope, id)
}

func (s *sectionService) ListByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingSection, error) {
	return s.sectionRepo.ListByPage(ctx, scope, pageID)
}

func (s *sectionService) Update(ctx context.Context, scope coretenant.Scope, organizationType coretenant.OrganizationType, id string, params repository.UpdateSectionParams) (domain.LandingSection, error) {
	if params.Content != nil {
		existing, err := s.sectionRepo.FindByID(ctx, scope, id)
		if err != nil {
			return domain.LandingSection{}, err
		}
		if err := validatePricingSource(existing.Type, params.Content, organizationType); err != nil {
			return domain.LandingSection{}, err
		}
		params.Content = s.sanitizeMap(params.Content)
	}
	return s.sectionRepo.Update(ctx, scope, id, params)
}

func (s *sectionService) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	return s.sectionRepo.Delete(ctx, scope, id)
}

func (s *sectionService) Toggle(ctx context.Context, scope coretenant.Scope, id string, isEnabled bool, updatedBy string) error {
	_, err := s.sectionRepo.Update(ctx, scope, id, repository.UpdateSectionParams{
		IsEnabled: &isEnabled,
		UpdatedBy: updatedBy,
	})
	return err
}

func (s *sectionService) Reorder(ctx context.Context, scope coretenant.Scope, pageID string, params []repository.SectionReorderParam) error {
	return s.sectionRepo.Reorder(ctx, scope, pageID, params)
}

var xssRegex = regexp.MustCompile(`(?i)<\/?(script|iframe|object|embed|applet|meta|link|style|base|form|input|button|textarea|select)[^>]*>`)

func (s *sectionService) sanitizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	sanitized := make(map[string]any)
	for k, v := range m {
		switch val := v.(type) {
		case string:
			// Simple XSS sanitization (removing dangerous tags)
			sanitized[k] = s.sanitizeString(val)
		case map[string]any:
			sanitized[k] = s.sanitizeMap(val)
		case []any:
			sanitized[k] = s.sanitizeSlice(val)
		default:
			sanitized[k] = val
		}
	}
	return sanitized
}

func (s *sectionService) sanitizeSlice(arr []any) []any {
	sanitized := make([]any, len(arr))
	for i, v := range arr {
		switch val := v.(type) {
		case string:
			sanitized[i] = s.sanitizeString(val)
		case map[string]any:
			sanitized[i] = s.sanitizeMap(val)
		case []any:
			sanitized[i] = s.sanitizeSlice(val)
		default:
			sanitized[i] = val
		}
	}
	return sanitized
}

func (s *sectionService) sanitizeString(str string) string {
	// Strip potentially dangerous tags
	clean := xssRegex.ReplaceAllString(str, "")
	
	// Handle basic "javascript:" links in hrefs or src attributes if any (simplified)
	if strings.Contains(strings.ToLower(clean), "javascript:") {
		clean = strings.ReplaceAll(clean, "javascript:", "blocked-js:")
		clean = strings.ReplaceAll(clean, "JavaScript:", "blocked-js:")
	}

	return clean
}
