package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var activationTestNow = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

// activationRepoFake indexes activations by (organization_id, token hash) the
// way the SQL does, and records the lookup key it was given.
type activationRepoFake struct {
	repository.ActivationRepository
	byHash     map[string]domain.LandingPageActivation
	lookedUp   []string
	lookupOrgs []string
}

func (f *activationRepoFake) FindByTokenHash(_ context.Context, scope coretenant.Scope, hash string) (domain.LandingPageActivation, error) {
	f.lookedUp = append(f.lookedUp, hash)
	f.lookupOrgs = append(f.lookupOrgs, scope.OrganizationID())
	a, ok := f.byHash[hash]
	if !ok || a.OrganizationID != scope.OrganizationID() {
		return domain.LandingPageActivation{}, pgx.ErrNoRows
	}
	return a, nil
}

type activationPageRepoFake struct {
	repository.PageRepository
	pages     map[string]domain.LandingPage
	requested []string
}

func (f *activationPageRepoFake) FindByID(_ context.Context, scope coretenant.Scope, id string) (domain.LandingPage, error) {
	f.requested = append(f.requested, id)
	p, ok := f.pages[id]
	if !ok || p.OrganizationID != scope.OrganizationID() {
		return domain.LandingPage{}, pgx.ErrNoRows
	}
	return p, nil
}

type activationResolverRepoStub struct {
	grapesResolverRepoStub
	page    domain.LandingPage
	pageErr error
}

func (s *activationResolverRepoStub) ResolveBySlug(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return s.page, s.pageErr
}

func (s *activationResolverRepoStub) ResolveByDomain(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return s.page, s.pageErr
}

func (s *activationResolverRepoStub) ResolveHomepage(context.Context, coretenant.Scope) (domain.LandingPage, error) {
	return s.page, s.pageErr
}

func newActivationResolver(resolverRepo repository.ResolverRepository, pages *activationPageRepoFake, acts *activationRepoFake) *resolverService {
	svc := NewResolverService(nil, resolverRepo, nil, pages, nil, &grapesFormRepoStub{}, nil, nil, grapesDocumentRepoStub{}, nil,
		WithResolverActivations(acts),
		WithResolverClock(func() time.Time { return activationTestNow }),
	)
	return svc.(*resolverService)
}

func grapesPublishedPage(activeUntil *time.Time) domain.LandingPage {
	return domain.LandingPage{
		ID: "page-a", OrganizationID: landingServiceOrganizationID, Slug: "undangan-budi",
		Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusPublished,
		Visibility: domain.PageVisibilityPublic, ActiveUntil: activeUntil,
	}
}

func TestResolveBySlug_PublishedWithPastActiveUntilIsNotFound(t *testing.T) {
	past := activationTestNow.Add(-time.Minute)
	for _, at := range []time.Time{past, activationTestNow} {
		repo := &activationResolverRepoStub{page: grapesPublishedPage(&at)}
		svc := newActivationResolver(repo, &activationPageRepoFake{}, &activationRepoFake{})

		_, err := svc.ResolveBySlug(context.Background(), mustLandingScope(t), "undangan-budi", "")
		if !errors.Is(err, ErrPageNotFound) {
			t.Fatalf("active_until=%s: err = %v, want ErrPageNotFound", at, err)
		}
		_, err = svc.ResolveByDomain(context.Background(), mustLandingScope(t), "budi.example.test", "")
		if !errors.Is(err, ErrPageNotFound) {
			t.Fatalf("domain, active_until=%s: err = %v, want ErrPageNotFound", at, err)
		}
	}
}

func TestResolveBySlug_ActiveUntilNullOrFutureIsServed(t *testing.T) {
	future := activationTestNow.Add(time.Hour)
	for name, page := range map[string]domain.LandingPage{
		"null (regression)": grapesPublishedPage(nil),
		"future":            grapesPublishedPage(&future),
	} {
		repo := &activationResolverRepoStub{page: page}
		svc := newActivationResolver(repo, &activationPageRepoFake{}, &activationRepoFake{})

		resolved, err := svc.ResolveBySlug(context.Background(), mustLandingScope(t), "undangan-budi", "")
		if err != nil {
			t.Fatalf("%s: err = %v", name, err)
		}
		if resolved.Page.ID != "page-a" || resolved.IsDraft {
			t.Fatalf("%s: resolved = %+v", name, resolved.Page)
		}
	}
}

func TestResolveHomepage_ExpiredHomepageFallsBack(t *testing.T) {
	past := activationTestNow.Add(-time.Minute)
	repo := &homepageFallbackRepo{home: grapesPublishedPage(&past), fallback: grapesPublishedPage(nil)}
	repo.fallback.ID = "fallback"
	svc := newActivationResolver(repo, &activationPageRepoFake{}, &activationRepoFake{})

	resolved, err := svc.ResolveHomepage(context.Background(), mustLandingScope(t))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resolved.Page.ID != "fallback" {
		t.Fatalf("expired homepage must not be served, got %q", resolved.Page.ID)
	}
}

type homepageFallbackRepo struct {
	grapesResolverRepoStub
	home, fallback domain.LandingPage
}

func (r *homepageFallbackRepo) ResolveHomepage(context.Context, coretenant.Scope) (domain.LandingPage, error) {
	return r.home, nil
}

func (r *homepageFallbackRepo) ResolveBySlug(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return r.fallback, nil
}

const otherOrganizationID = "33333333-3333-3333-3333-333333333333"

func previewFixture(t *testing.T) (raw string, acts *activationRepoFake, pages *activationPageRepoFake) {
	t.Helper()
	raw, hash, err := domain.NewActivationPreviewToken()
	if err != nil {
		t.Fatal(err)
	}
	acts = &activationRepoFake{byHash: map[string]domain.LandingPageActivation{
		hash: {
			ID: "act-a", OrganizationID: landingServiceOrganizationID, PageID: "page-a",
			Status: domain.ActivationStatusOpen, InvoiceDueAt: activationTestNow.Add(time.Hour),
			PreviewTokenHash: hash,
		},
	}}
	draft := grapesPublishedPage(nil)
	draft.Status = domain.PageStatusDraft
	other := grapesPublishedPage(nil)
	other.ID, other.Slug, other.Status = "page-b", "lain", domain.PageStatusDraft
	pages = &activationPageRepoFake{pages: map[string]domain.LandingPage{"page-a": draft, "page-b": other}}
	return raw, acts, pages
}

func TestResolveActivationPreview_ValidTokenServesItsPageAsDraft(t *testing.T) {
	raw, acts, pages := previewFixture(t)
	svc := newActivationResolver(&grapesResolverRepoStub{}, pages, acts)

	resolved, err := svc.ResolveActivationPreview(context.Background(), mustLandingScope(t), raw)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resolved.Page.ID != "page-a" || !resolved.IsDraft {
		t.Fatalf("resolved page = %q draft=%v", resolved.Page.ID, resolved.IsDraft)
	}
	if len(pages.requested) != 1 || pages.requested[0] != "page-a" {
		t.Fatalf("page lookup must be the activation's page only, got %v", pages.requested)
	}
}

func TestResolveActivationPreview_LooksUpHashNotRawToken(t *testing.T) {
	raw, acts, pages := previewFixture(t)
	svc := newActivationResolver(&grapesResolverRepoStub{}, pages, acts)

	_, _ = svc.ResolveActivationPreview(context.Background(), mustLandingScope(t), raw)
	if len(acts.lookedUp) != 1 {
		t.Fatalf("lookups = %v", acts.lookedUp)
	}
	if acts.lookedUp[0] == raw || strings.Contains(acts.lookedUp[0], raw) {
		t.Fatal("repository must receive the token hash, never the raw token")
	}
	if acts.lookedUp[0] != domain.HashActivationPreviewToken(raw) {
		t.Fatalf("lookup key = %q, want sha256 hex of token", acts.lookedUp[0])
	}
}

func TestResolveActivationPreview_RejectsClosedOrForeignTokens(t *testing.T) {
	cases := map[string]func(acts *activationRepoFake, raw *string, scope *coretenant.Scope, t *testing.T){
		"unknown token": func(_ *activationRepoFake, raw *string, _ *coretenant.Scope, _ *testing.T) {
			*raw = "tidak-dikenal"
		},
		"past invoice due": func(acts *activationRepoFake, raw *string, _ *coretenant.Scope, _ *testing.T) {
			mutate(acts, *raw, func(a *domain.LandingPageActivation) { a.InvoiceDueAt = activationTestNow.Add(-time.Second) })
		},
		"due exactly now": func(acts *activationRepoFake, raw *string, _ *coretenant.Scope, _ *testing.T) {
			mutate(acts, *raw, func(a *domain.LandingPageActivation) { a.InvoiceDueAt = activationTestNow })
		},
		"void": func(acts *activationRepoFake, raw *string, _ *coretenant.Scope, _ *testing.T) {
			mutate(acts, *raw, func(a *domain.LandingPageActivation) { a.Status = domain.ActivationStatusVoid })
		},
		"paid": func(acts *activationRepoFake, raw *string, _ *coretenant.Scope, _ *testing.T) {
			mutate(acts, *raw, func(a *domain.LandingPageActivation) { a.Status = domain.ActivationStatusPaid })
		},
		"other tenant host": func(_ *activationRepoFake, _ *string, scope *coretenant.Scope, t *testing.T) {
			*scope = scopeFor(t, otherOrganizationID)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			raw, acts, pages := previewFixture(t)
			scope := mustLandingScope(t)
			setup(acts, &raw, &scope, t)
			svc := newActivationResolver(&grapesResolverRepoStub{}, pages, acts)

			_, err := svc.ResolveActivationPreview(context.Background(), scope, raw)
			if !errors.Is(err, ErrPageNotFound) {
				t.Fatalf("err = %v, want ErrPageNotFound", err)
			}
			if name == "unknown token" || name == "other tenant host" {
				if !errors.Is(err, ErrActivationTokenNotFound) {
					t.Fatalf("err = %v, want ErrActivationTokenNotFound", err)
				}
			}
			if len(pages.requested) != 0 {
				t.Fatalf("no page may be loaded for a rejected token, got %v", pages.requested)
			}
		})
	}
}

func TestResolveActivationPreview_PageOfAnotherOrganizationIsNotServed(t *testing.T) {
	raw, acts, pages := previewFixture(t)
	p := pages.pages["page-a"]
	p.OrganizationID = otherOrganizationID
	pages.pages["page-a"] = p
	svc := newActivationResolver(&grapesResolverRepoStub{}, pages, acts)

	if _, err := svc.ResolveActivationPreview(context.Background(), mustLandingScope(t), raw); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("err = %v, want ErrPageNotFound", err)
	}
}

func TestResolveActivationPreview_ExpiredPageRenewalIsPreviewable(t *testing.T) {
	raw, acts, pages := previewFixture(t)
	past := activationTestNow.Add(-24 * time.Hour)
	p := pages.pages["page-a"]
	p.Status, p.ActiveUntil = domain.PageStatusPublished, &past
	pages.pages["page-a"] = p
	svc := newActivationResolver(&grapesResolverRepoStub{}, pages, acts)

	resolved, err := svc.ResolveActivationPreview(context.Background(), mustLandingScope(t), raw)
	if err != nil || resolved.Page.ID != "page-a" {
		t.Fatalf("renewal preview of an expired page must be served: page=%q err=%v", resolved.Page.ID, err)
	}
}

func mutate(acts *activationRepoFake, raw string, fn func(*domain.LandingPageActivation)) {
	hash := domain.HashActivationPreviewToken(raw)
	a := acts.byHash[hash]
	fn(&a)
	acts.byHash[hash] = a
}

func scopeFor(t *testing.T, orgID string) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     orgID,
		OrganizationSlug:   "lain",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceCustomDomain,
		DataPlacement:      coretenant.DataPlacementShared,
		RequestHost:        "lain.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
