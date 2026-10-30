package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database"
)

var (
	ErrPageNotPublished = errors.New("landing page is not published")
	// ErrActivationTokenNotFound means no activation of the host organization
	// has this preview token. It wraps ErrPageNotFound so callers that only
	// care about "not found" (404) need no extra case.
	ErrActivationTokenNotFound = fmt.Errorf("activation preview token not found: %w", ErrPageNotFound)
)

// ResolverOption configures optional resolver dependencies.
type ResolverOption func(*resolverService)

// WithResolverActivations enables activation preview links
// (GET /public/landing/preview/:token).
func WithResolverActivations(repo repository.ActivationRepository) ResolverOption {
	return func(s *resolverService) { s.activationRepo = repo }
}

// WithResolverClock overrides the clock used for active_until / invoice_due_at
// comparisons (tests). The default is time.Now in UTC.
func WithResolverClock(now func() time.Time) ResolverOption {
	return func(s *resolverService) { s.now = now }
}

type resolverService struct {
	db             *database.Pool
	resolverRepo   repository.ResolverRepository
	versionRepo    repository.VersionRepository
	pageRepo       repository.PageRepository
	sectionRepo    repository.SectionRepository
	formRepo       repository.FormRepository
	brandingRepo   repository.BrandingRepository
	reusableRepo   repository.ReusableRepository
	documentRepo   repository.DocumentRepository
	publishService PublishService
	activationRepo repository.ActivationRepository
	now            func() time.Time
}

func NewResolverService(
	db *database.Pool,
	resolverRepo repository.ResolverRepository,
	versionRepo repository.VersionRepository,
	pageRepo repository.PageRepository,
	sectionRepo repository.SectionRepository,
	formRepo repository.FormRepository,
	brandingRepo repository.BrandingRepository,
	reusableRepo repository.ReusableRepository,
	documentRepo repository.DocumentRepository,
	publishService PublishService,
	opts ...ResolverOption,
) ResolverService {
	s := &resolverService{
		db:             db,
		resolverRepo:   resolverRepo,
		versionRepo:    versionRepo,
		pageRepo:       pageRepo,
		sectionRepo:    sectionRepo,
		formRepo:       formRepo,
		brandingRepo:   brandingRepo,
		reusableRepo:   reusableRepo,
		documentRepo:   documentRepo,
		publishService: publishService,
		now:            func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *resolverService) ResolveBySlug(ctx context.Context, scope tenant.Scope, slug string, previewToken string) (ResolvedPage, error) {
	page, err := s.resolverRepo.ResolveBySlug(ctx, scope, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedPage{}, ErrPageNotFound
		}
		return ResolvedPage{}, err
	}

	return s.resolvePageData(ctx, scope, page, previewToken)
}

// fallbackHomepageSlug is the legacy platform marketing page seeded by
// migration 000035; it keeps "/" alive when no is_homepage page is published.
const fallbackHomepageSlug = "public-marketing"

func (s *resolverService) ResolveHomepage(ctx context.Context, scope tenant.Scope) (ResolvedPage, error) {
	page, err := s.resolverRepo.ResolveHomepage(ctx, scope)
	switch {
	case err == nil:
		resolved, resolveErr := s.resolvePageData(ctx, scope, page, "")
		if resolveErr == nil {
			return resolved, nil
		}
		if !errors.Is(resolveErr, ErrPageNotPublished) && !errors.Is(resolveErr, ErrPageNotFound) {
			// Unlike "not published", an unexpected error means a configured
			// homepage is broken; falling back keeps "/" alive but must not be silent.
			slog.Warn("landing homepage resolve failed, falling back to default page",
				"page_id", page.ID,
				"organization_id", scope.OrganizationID(),
				"error_type", fmt.Sprintf("%T", resolveErr),
			)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return ResolvedPage{}, err
	}

	return s.ResolveBySlug(ctx, scope, fallbackHomepageSlug, "")
}

func (s *resolverService) ResolveByDomain(ctx context.Context, scope tenant.Scope, customDomain string, previewToken string) (ResolvedPage, error) {
	page, err := s.resolverRepo.ResolveByDomain(ctx, scope, customDomain)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedPage{}, ErrPageNotFound
		}
		return ResolvedPage{}, err
	}

	return s.resolvePageData(ctx, scope, page, previewToken)
}

// ResolveActivationPreview serves the single page an activation preview token
// was issued for, while the activation is open and its invoice not yet due.
// The page is chosen by the token only; the token is looked up by its sha256
// within the host organization, so a token never opens another page or a page
// of another tenant. The working copy is served (IsDraft), also for a renewal
// of an expired page.
func (s *resolverService) ResolveActivationPreview(ctx context.Context, scope tenant.Scope, token string) (ResolvedPage, error) {
	if s.activationRepo == nil || token == "" {
		return ResolvedPage{}, ErrActivationTokenNotFound
	}
	activation, err := s.activationRepo.FindByTokenHash(ctx, scope, domain.HashActivationPreviewToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedPage{}, ErrActivationTokenNotFound
		}
		return ResolvedPage{}, err
	}
	if activation.OrganizationID != scope.OrganizationID() {
		return ResolvedPage{}, ErrActivationTokenNotFound
	}
	if !activation.PreviewableAt(s.now()) {
		return ResolvedPage{}, ErrPageNotFound
	}

	page, err := s.pageRepo.FindByID(ctx, scope, activation.PageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedPage{}, ErrPageNotFound
		}
		return ResolvedPage{}, err
	}
	// FindByID also returns platform templates; only the tenant's own page counts.
	if page.ID != activation.PageID || page.OrganizationID != scope.OrganizationID() || page.IsTemplate {
		return ResolvedPage{}, ErrPageNotFound
	}
	return s.resolveDraftPage(ctx, scope, page), nil
}

func (s *resolverService) resolvePageData(ctx context.Context, scope tenant.Scope, page domain.LandingPage, previewToken string) (ResolvedPage, error) {
	// A sold page whose publish period is over is hidden like a missing page
	// (not deleted). Pages never activated (active_until NULL) are unaffected.
	if domain.PageActivationExpired(page, s.now()) {
		return ResolvedPage{}, ErrPageNotFound
	}

	isDraftPreview := false

	if page.Status != domain.PageStatusPublished {
		if previewToken == "" {
			return ResolvedPage{}, ErrPageNotPublished
		}

		// Validate token
		validPageID, err := s.publishService.ValidatePreviewToken(ctx, previewToken)
		if err != nil || validPageID != page.ID {
			return ResolvedPage{}, ErrPageNotPublished
		}
		isDraftPreview = true
	}

	if page.Builder == domain.PageBuilderGrapesJS {
		return s.resolveGrapesPage(ctx, scope, page, isDraftPreview), nil
	}

	if isDraftPreview {
		return s.resolveDraftPage(ctx, scope, page), nil
	}

	// Fetch Published Snapshot
	versions, err := s.resolverRepo.ResolveVersions(ctx, scope, page.ID)
	if err != nil || len(versions) == 0 {
		// If no versions found but status is published, fallback to active working data (should not happen normally)
		sections := s.resolveSections(ctx, scope, page.ID)
		forms := s.resolveForms(ctx, scope, page.ID)
		branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
		if branding.ID == "" {
			branding, _ = s.brandingRepo.GetDefault(ctx, scope)
		}
		return ResolvedPage{
			Page:         page,
			Sections:     sections,
			Forms:        forms,
			Branding:     branding,
			Menus:        s.resolveMenus(ctx, scope),
			CTAs:         s.resolveCTAs(ctx, scope),
			PricingPlans: s.resolvePricingPlans(ctx, scope),
		}, nil
	}

	latestVersion := versions[0]
	sections := []domain.LandingSection{}
	forms := []domain.LandingForm{}

	if !snapshotHasItems(latestVersion.Snapshot, "sections") {
		sections = s.resolveSections(ctx, scope, page.ID)
	}
	if !snapshotHasItems(latestVersion.Snapshot, "forms") {
		forms = s.resolveForms(ctx, scope, page.ID)
	}

	// Also fetch branding (branding is usually dynamic and not always fully snapshotted or applied globally)
	branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
	if branding.ID == "" {
		branding, _ = s.brandingRepo.GetDefault(ctx, scope)
	}

	return ResolvedPage{
		Page:         page,
		Sections:     sections,
		Forms:        forms,
		Snapshot:     latestVersion.Snapshot,
		Branding:     branding,
		Menus:        s.resolveMenus(ctx, scope),
		CTAs:         s.resolveCTAs(ctx, scope),
		PricingPlans: s.resolvePricingPlans(ctx, scope),
		IsDraft:      false,
	}, nil
}

// resolveDraftPage serves the page's working copy (draft preview).
func (s *resolverService) resolveDraftPage(ctx context.Context, scope tenant.Scope, page domain.LandingPage) ResolvedPage {
	if page.Builder == domain.PageBuilderGrapesJS {
		return s.resolveGrapesPage(ctx, scope, page, true)
	}
	sections := s.resolveSections(ctx, scope, page.ID)
	forms := s.resolveForms(ctx, scope, page.ID)
	branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
	if branding.ID == "" {
		branding, _ = s.brandingRepo.GetDefault(ctx, scope)
	}

	return ResolvedPage{
		Page:         page,
		Sections:     sections,
		Forms:        forms,
		Branding:     branding,
		Menus:        s.resolveMenus(ctx, scope),
		CTAs:         s.resolveCTAs(ctx, scope),
		PricingPlans: s.resolvePricingPlans(ctx, scope),
		IsDraft:      true,
	}
}

// resolveGrapesPage serves a GrapesJS page as sanitized HTML+CSS. Menus and
// branding stay live (tenant chrome is substituted client-side, Phase 5b).
func (s *resolverService) resolveGrapesPage(ctx context.Context, scope tenant.Scope, page domain.LandingPage, isDraftPreview bool) ResolvedPage {
	branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
	if branding.ID == "" {
		branding, _ = s.brandingRepo.GetDefault(ctx, scope)
	}

	resolved := ResolvedPage{
		Page:         page,
		Builder:      string(domain.PageBuilderGrapesJS),
		Branding:     branding,
		Menus:        s.resolveMenus(ctx, scope),
		CTAs:         s.resolveCTAs(ctx, scope),
		PricingPlans: s.resolvePricingPlans(ctx, scope),
		IsDraft:      isDraftPreview,
		Forms:        s.resolveActiveFormsWithFields(ctx, scope, page.ID),
	}

	if !isDraftPreview {
		if versions, err := s.resolverRepo.ResolveVersions(ctx, scope, page.ID); err == nil && len(versions) > 0 {
			resolved.Snapshot = versions[0].Snapshot
			resolved.HTML, resolved.CSS = grapesSnapshotMarkup(versions[0].Snapshot)
			return resolved
		}
		// Published but no snapshot — fall through to the live document below.
	}

	// Draft preview (or the no-snapshot fallback): the working copy is
	// untrusted-ish, so sanitize on read.
	doc, err := s.documentRepo.GetByPageID(ctx, scope, page.ID)
	if err == nil {
		resolved.HTML = SanitizeGrapesHTML(doc.HTML)
		resolved.CSS = SanitizeGrapesCSS(doc.CSS)
	}
	return resolved
}

func (s *resolverService) resolveSections(ctx context.Context, scope tenant.Scope, pageID string) []domain.LandingSection {
	sections, err := s.resolverRepo.ResolveSections(ctx, scope, pageID)
	if err != nil {
		return []domain.LandingSection{}
	}
	return sections
}

// resolveActiveFormsWithFields returns the page's active forms with their
// fields, which is what a GrapesJS lead-form block needs to render. Inactive
// forms are left out so the public page never offers a form that would reject
// the submission.
func (s *resolverService) resolveActiveFormsWithFields(ctx context.Context, scope tenant.Scope, pageID string) []domain.LandingForm {
	all := s.resolveForms(ctx, scope, pageID)
	active := make([]domain.LandingForm, 0, len(all))
	for _, form := range all {
		if !form.IsActive {
			continue
		}
		if fields, err := s.formRepo.ListFieldsByForm(ctx, scope, form.ID); err == nil {
			form.Fields = withoutDefaultValues(fields)
		}
		active = append(active, form)
	}
	return active
}

func (s *resolverService) resolveForms(ctx context.Context, scope tenant.Scope, pageID string) []domain.LandingForm {
	forms, err := s.resolverRepo.ResolveForms(ctx, scope, pageID)
	if err != nil {
		return []domain.LandingForm{}
	}
	// The public payload must not reveal the CRM routing config (PIC user id,
	// lead flag); the admin API returns it from the form service instead.
	for i := range forms {
		forms[i].LeadOwnerUserID = ""
		forms[i].CreateCRMLead = false
	}
	return forms
}

// withoutDefaultValues returns a copy of fields with DefaultValue cleared. The default of a hidden
// field (for example an internal campaign tag) is applied server-side on submit and must never
// reach the public payload.
func withoutDefaultValues(fields []domain.LandingFormField) []domain.LandingFormField {
	out := make([]domain.LandingFormField, len(fields))
	for i, f := range fields {
		f.DefaultValue = ""
		out[i] = f
	}
	return out
}

func snapshotHasItems(snapshot map[string]any, key string) bool {
	if snapshot == nil {
		return false
	}

	value, exists := snapshot[key]
	if !exists || value == nil {
		return false
	}

	switch items := value.(type) {
	case []any:
		return len(items) > 0
	case []domain.LandingSection:
		return len(items) > 0
	case []domain.LandingForm:
		return len(items) > 0
	default:
		return true
	}
}

func (s *resolverService) resolveCTAs(ctx context.Context, scope tenant.Scope) []ResolvedCTA {
	if s.reusableRepo == nil {
		return nil
	}
	ctas, err := s.reusableRepo.ListCTAs(ctx, scope)
	if err != nil {
		return nil
	}
	resolved := make([]ResolvedCTA, 0, len(ctas))
	for _, cta := range ctas {
		resolved = append(resolved, ResolvedCTA{
			ID:          cta.ID,
			Label:       cta.Label,
			Target:      string(cta.Target),
			Destination: cta.Destination,
			TrackingKey: cta.TrackingKey,
		})
	}
	return resolved
}

func (s *resolverService) resolvePricingPlans(ctx context.Context, scope tenant.Scope) []ResolvedPricingPlan {
	if s.reusableRepo == nil {
		return nil
	}
	plans, err := s.reusableRepo.ListPricingPlans(ctx, scope)
	if err != nil {
		return nil
	}
	resolved := make([]ResolvedPricingPlan, 0, len(plans))
	for _, plan := range plans {
		if !plan.IsEnabled {
			continue
		}
		features := plan.Features
		if features == nil {
			features = []string{}
		}
		resolved = append(resolved, ResolvedPricingPlan{
			ID:            plan.ID,
			Name:          plan.Name,
			PriceLabel:    plan.PriceLabel,
			IntervalLabel: plan.IntervalLabel,
			Description:   plan.Description,
			Features:      features,
			CTALabel:      plan.CTALabel,
			CTAURL:        plan.CTAURL,
			IsFeatured:    plan.IsFeatured,
		})
	}
	return resolved
}

func (s *resolverService) resolveMenus(ctx context.Context, scope tenant.Scope) []ResolvedMenu {
	menus, err := s.resolverRepo.ResolveMenus(ctx, scope)
	if err != nil {
		return nil
	}

	resolvedMenus := make([]ResolvedMenu, 0, len(menus))
	for _, menu := range menus {
		items, err := s.resolverRepo.ResolveMenuItems(ctx, scope, menu.ID)
		if err != nil {
			continue
		}
		resolvedMenus = append(resolvedMenus, ResolvedMenu{
			ID:       menu.ID,
			Name:     menu.Name,
			Location: string(menu.Location),
			IsActive: menu.IsActive,
			Items:    buildResolvedMenuTree(items),
		})
	}

	return resolvedMenus
}

func buildResolvedMenuTree(items []domain.LandingMenuItem) []ResolvedMenuItem {
	byID := make(map[string]*ResolvedMenuItem, len(items))
	orderedIDs := make([]string, 0, len(items))

	for _, item := range items {
		resolved := ResolvedMenuItem{
			ID:          item.ID,
			ParentID:    item.ParentID,
			Label:       item.Label,
			LinkType:    string(item.LinkType),
			Destination: item.Destination,
			Target:      string(item.Target),
			SortOrder:   item.SortOrder,
			IsEnabled:   item.IsEnabled,
		}
		byID[item.ID] = &resolved
		orderedIDs = append(orderedIDs, item.ID)
	}

	roots := make([]ResolvedMenuItem, 0, len(items))
	for _, id := range orderedIDs {
		item := byID[id]
		if item.ParentID != nil {
			if parent := byID[*item.ParentID]; parent != nil {
				parent.Children = append(parent.Children, *item)
				continue
			}
		}
		roots = append(roots, *item)
	}

	return roots
}
