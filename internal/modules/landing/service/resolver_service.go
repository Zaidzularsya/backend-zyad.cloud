package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database"
)

var (
	ErrPageNotPublished = errors.New("landing page is not published")
)

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
) ResolverService {
	return &resolverService{
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
	}
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

func (s *resolverService) resolvePageData(ctx context.Context, scope tenant.Scope, page domain.LandingPage, previewToken string) (ResolvedPage, error) {
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
		// Fetch active working data
		sections := s.resolveSections(ctx, scope, page.ID)
		forms := s.resolveForms(ctx, scope, page.ID)
		branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
		if branding.ID == "" {
			branding, _ = s.brandingRepo.GetDefault(ctx, scope)
		}

		return ResolvedPage{
			Page:     page,
			Sections: sections,
			Forms:    forms,
			Branding: branding,
			Menus:    s.resolveMenus(ctx, scope),
			CTAs:     s.resolveCTAs(ctx, scope),
			IsDraft:  true,
		}, nil
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
			Page:     page,
			Sections: sections,
			Forms:    forms,
			Branding: branding,
			Menus:    s.resolveMenus(ctx, scope),
			CTAs:     s.resolveCTAs(ctx, scope),
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
		Page:     page,
		Sections: sections,
		Forms:    forms,
		Snapshot: latestVersion.Snapshot,
		Branding: branding,
		Menus:    s.resolveMenus(ctx, scope),
		CTAs:     s.resolveCTAs(ctx, scope),
		IsDraft:  false,
	}, nil
}

// resolveGrapesPage serves a GrapesJS page as sanitized HTML+CSS. Menus and
// branding stay live (tenant chrome is substituted client-side, Phase 5b).
func (s *resolverService) resolveGrapesPage(ctx context.Context, scope tenant.Scope, page domain.LandingPage, isDraftPreview bool) ResolvedPage {
	branding, _ := s.resolverRepo.ResolveBranding(ctx, scope, page.ID)
	if branding.ID == "" {
		branding, _ = s.brandingRepo.GetDefault(ctx, scope)
	}

	resolved := ResolvedPage{
		Page:     page,
		Builder:  string(domain.PageBuilderGrapesJS),
		Branding: branding,
		Menus:    s.resolveMenus(ctx, scope),
		CTAs:     s.resolveCTAs(ctx, scope),
		IsDraft:  isDraftPreview,
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

func (s *resolverService) resolveForms(ctx context.Context, scope tenant.Scope, pageID string) []domain.LandingForm {
	forms, err := s.resolverRepo.ResolveForms(ctx, scope, pageID)
	if err != nil {
		return []domain.LandingForm{}
	}
	return forms
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
