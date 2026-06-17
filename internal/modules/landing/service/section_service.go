package service

import (
	"context"
	"regexp"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type sectionService struct {
	sectionRepo repository.SectionRepository
}

func NewSectionService(sectionRepo repository.SectionRepository) SectionService {
	return &sectionService{
		sectionRepo: sectionRepo,
	}
}

func (s *sectionService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateSectionParams) (domain.LandingSection, error) {
	// Sanitize content
	params.Content = s.sanitizeMap(params.Content)
	return s.sectionRepo.Create(ctx, scope, params)
}

func (s *sectionService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSection, error) {
	return s.sectionRepo.FindByID(ctx, scope, id)
}

func (s *sectionService) ListByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingSection, error) {
	return s.sectionRepo.ListByPage(ctx, scope, pageID)
}

func (s *sectionService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateSectionParams) (domain.LandingSection, error) {
	if params.Content != nil {
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
