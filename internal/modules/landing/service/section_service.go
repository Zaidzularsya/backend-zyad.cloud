package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/microcosm-cc/bluemonday"

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

// footerVariants is the FTR-0002 v1 catalog. Variant is persisted as
// style.variant (no dedicated column, see footer-management-traceability-index.md).
var footerVariants = map[string]bool{
	"":           true, // backward compatible: resolves to "default" at render time
	"default":    true,
	"simple":     true,
	"newsletter": true,
	"mega":       true,
}

func errFooterVariantNotAllowed(variant string) error {
	return coreerrors.New(
		"VALIDATION_ERROR",
		"unknown footer variant \""+variant+"\", expected one of: default, simple, newsletter, mega",
		http.StatusUnprocessableEntity,
	)
}

func validateFooterVariant(sectionType domain.SectionType, style map[string]any) error {
	if sectionType != domain.SectionTypeFooter || style == nil {
		return nil
	}
	variant, ok := style["variant"].(string)
	if !ok {
		return nil
	}
	if !footerVariants[variant] {
		return errFooterVariantNotAllowed(variant)
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
	if err := validateFooterVariant(params.Type, params.Style); err != nil {
		return domain.LandingSection{}, err
	}
	if err := s.requireCreateQuota(ctx, scope, params.LandingPageID); err != nil {
		return domain.LandingSection{}, err
	}
	// Sanitize content + style
	params.Content = s.sanitizeMap(params.Content)
	params.Style = s.sanitizeStyleMap(params.Style)
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
	if params.Content != nil || params.Style != nil {
		existing, err := s.sectionRepo.FindByID(ctx, scope, id)
		if err != nil {
			return domain.LandingSection{}, err
		}
		if err := validatePricingSource(existing.Type, params.Content, organizationType); err != nil {
			return domain.LandingSection{}, err
		}
		if err := validateFooterVariant(existing.Type, params.Style); err != nil {
			return domain.LandingSection{}, err
		}
		if params.Content != nil {
			params.Content = s.sanitizeMap(params.Content)
		}
		if params.Style != nil {
			params.Style = s.sanitizeStyleMap(params.Style)
		}
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

func (s *sectionService) ReplaceAll(
	ctx context.Context,
	scope coretenant.Scope,
	organizationType coretenant.OrganizationType,
	pageID string,
	items []repository.ReplaceSectionItem,
	actorID string,
) ([]domain.LandingSection, error) {
	sanitized := make([]repository.ReplaceSectionItem, len(items))
	for i, item := range items {
		if err := validatePricingSource(item.Type, item.Content, organizationType); err != nil {
			return nil, err
		}
		if err := validateFooterVariant(item.Type, item.Style); err != nil {
			return nil, err
		}

		content := s.sanitizeMap(item.Content)
		if content == nil {
			content = map[string]any{}
		}
		style := s.sanitizeStyleMap(item.Style)
		if style == nil {
			style = map[string]any{}
		}

		item.Content = content
		item.Style = style
		sanitized[i] = item
	}

	if s.quotaGuard != nil {
		if err := s.quotaGuard.RequireQuotaValue(
			ctx,
			scope.OrganizationID(),
			domain.FeatureLandingMaxSections,
			"limit",
			int64(len(sanitized)),
			0,
		); err != nil {
			return nil, err
		}
	}

	return s.sectionRepo.ReplaceAll(ctx, scope, repository.ReplaceAllParams{
		LandingPageID: pageID,
		ActorID:       actorID,
		Items:         sanitized,
	})
}

// contentSanitizerPolicy is the single allowlist used for every string value stored in
// section content: a small set of inline formatting tags plus links restricted to
// standard http/https/mailto URLs. Anything else (script/event handlers/style/svg/etc.)
// is stripped, not just a denylist of known-dangerous tags.
var contentSanitizerPolicy = newContentSanitizerPolicy()

func newContentSanitizerPolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements("b", "i", "em", "strong", "u", "br", "span")
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowElements("a")
	policy.AllowStandardURLs()
	policy.RequireNoFollowOnLinks(true)
	return policy
}

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
	return contentSanitizerPolicy.Sanitize(str)
}

// styleTextPolicy strips every tag/entity from a style string leaf. style values
// are class names, colour hexes, enum tokens and the like — never markup.
var styleTextPolicy = bluemonday.StrictPolicy()

// sanitizeStyleMap drops any top-level key outside domain.AllowedStyleKeys and
// recursively cleans the rest: URL-ish leaves go through sanitizeStyleURL, other
// string leaves are stripped of markup, numbers/bools are left as-is.
func (s *sectionService) sanitizeStyleMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	sanitized := make(map[string]any)
	for k, v := range m {
		if !domain.AllowedStyleKeys[k] {
			continue
		}
		sanitized[k] = s.sanitizeStyleValue(k, v)
	}
	return sanitized
}

func (s *sectionService) sanitizeStyleValue(key string, v any) any {
	switch val := v.(type) {
	case string:
		if domain.StyleURLKeys[key] {
			return sanitizeStyleURL(val)
		}
		return styleTextPolicy.Sanitize(val)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, nv := range val {
			out[k] = s.sanitizeStyleValue(k, nv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, nv := range val {
			out[i] = s.sanitizeStyleValue(key, nv)
		}
		return out
	default:
		return val
	}
}

// sanitizeStyleURL returns raw only if it is a plain http(s) URL or a
// site-relative path with no characters that could break out of a CSS
// url("…") context; otherwise it returns "".
func sanitizeStyleURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, "\"'()<>;\\ \t\r\n") {
		return ""
	}
	if strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") {
		return v
	}
	parsed, err := url.Parse(v)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return v
}
