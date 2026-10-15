package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type ptRepoStub struct {
	items        map[string]domain.PageTemplate
	listFilter   repository.PageTemplateListFilter
	createParams repository.CreatePageTemplateParams
	createErr    error
	updateParams repository.UpdatePageTemplateParams
	updateCalls  int
	calls        int
}

func (s *ptRepoStub) List(_ context.Context, f repository.PageTemplateListFilter) ([]domain.PageTemplate, error) {
	s.calls++
	s.listFilter = f
	return nil, nil
}
func (s *ptRepoStub) FindByID(_ context.Context, id string) (domain.PageTemplate, error) {
	s.calls++
	t, ok := s.items[id]
	if !ok {
		return domain.PageTemplate{}, pgx.ErrNoRows
	}
	return t, nil
}
func (s *ptRepoStub) FindBySlug(context.Context, string) (domain.PageTemplate, error) {
	s.calls++
	return domain.PageTemplate{}, pgx.ErrNoRows
}
func (s *ptRepoStub) Create(_ context.Context, p repository.CreatePageTemplateParams) (domain.PageTemplate, error) {
	s.calls++
	s.createParams = p
	if s.createErr != nil {
		return domain.PageTemplate{}, s.createErr
	}
	return domain.PageTemplate{ID: "new", Slug: p.Slug, Status: p.Status, PlatformOnly: p.PlatformOnly, HTML: p.HTML}, nil
}
func (s *ptRepoStub) Update(_ context.Context, id string, p repository.UpdatePageTemplateParams) (domain.PageTemplate, error) {
	s.calls++
	s.updateCalls++
	s.updateParams = p
	t := s.items[id]
	return t, nil
}
func (s *ptRepoStub) WithSeedLock(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type ptDocStub struct {
	doc domain.LandingPageDocument
	err error
	up  repository.UpsertDocumentParams
}

func (s *ptDocStub) GetByPageID(context.Context, coretenant.Scope, string) (domain.LandingPageDocument, error) {
	return s.doc, s.err
}
func (s *ptDocStub) Upsert(_ context.Context, _ coretenant.Scope, p repository.UpsertDocumentParams) (domain.LandingPageDocument, error) {
	s.up = p
	return domain.LandingPageDocument{}, nil
}

type ptPageStub struct {
	documentPageRepoStub
	page         domain.LandingPage
	findErr      error
	takenSlugs   map[string]bool
	createParams repository.CreatePageParams
	created      bool
}

func (s *ptPageStub) FindByID(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return s.page, s.findErr
}
func (s *ptPageStub) FindBySlug(_ context.Context, _ coretenant.Scope, slug string) (domain.LandingPage, error) {
	if s.takenSlugs[slug] {
		return domain.LandingPage{ID: "x"}, nil
	}
	return domain.LandingPage{}, pgx.ErrNoRows
}
func (s *ptPageStub) Create(_ context.Context, _ coretenant.Scope, p repository.CreatePageParams) (domain.LandingPage, error) {
	s.created = true
	s.createParams = p
	return domain.LandingPage{ID: "page-new", Slug: p.Slug}, nil
}

func ptViewer(t *testing.T, platform bool) PageTemplateViewer {
	return PageTemplateViewer{Scope: mustLandingScope(t), IsPlatform: platform, ActorID: "user-1"}
}

func ptAssertCode(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func newPT(repo *ptRepoStub, docs *ptDocStub, pages *ptPageStub) PageTemplateService {
	return NewPageTemplateService(repo, docs, pages)
}

func TestPageTemplateService_TenantCannotSeeHidden(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{
		"draft":    {ID: "draft", Status: domain.PageTemplateStatusDraft},
		"platform": {ID: "platform", Status: domain.PageTemplateStatusPublished, PlatformOnly: true},
		"archived": {ID: "archived", Status: domain.PageTemplateStatusArchived},
	}}
	svc := newPT(repo, &ptDocStub{}, &ptPageStub{})
	for id := range repo.items {
		_, err := svc.Get(context.Background(), ptViewer(t, false), id)
		ptAssertCode(t, err, ErrPageTemplateNotFound)
	}
	_, err := svc.Get(context.Background(), ptViewer(t, false), "missing")
	ptAssertCode(t, err, ErrPageTemplateNotFound)
}

func TestPageTemplateService_PlatformGetsAnyStatus(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{
		"draft":    {ID: "draft", Status: domain.PageTemplateStatusDraft},
		"archived": {ID: "archived", Status: domain.PageTemplateStatusArchived},
	}}
	svc := newPT(repo, &ptDocStub{}, &ptPageStub{})
	for id := range repo.items {
		got, err := svc.Get(context.Background(), ptViewer(t, true), id)
		if err != nil || got.ID != id {
			t.Fatalf("platform Get(%s): %v %v", id, got.ID, err)
		}
	}
}

func TestPageTemplateService_ListPublishedFilter(t *testing.T) {
	for _, platform := range []bool{false, true} {
		repo := &ptRepoStub{}
		_, err := newPT(repo, &ptDocStub{}, &ptPageStub{}).ListPublished(context.Background(), ptViewer(t, platform))
		if err != nil {
			t.Fatal(err)
		}
		f := repo.listFilter
		if len(f.Statuses) != 1 || f.Statuses[0] != domain.PageTemplateStatusPublished || f.IncludePlatformOnly != platform {
			t.Errorf("platform=%v filter=%+v", platform, f)
		}
	}
}

func TestPageTemplateService_WritesRequirePlatform(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{"a": {ID: "a"}}}
	pages := &ptPageStub{}
	svc := newPT(repo, &ptDocStub{}, pages)
	v := ptViewer(t, false)
	ctx := context.Background()

	_, err := svc.ListAll(ctx, v)
	ptAssertCode(t, err, ErrPageTemplatePlatformOnly)
	_, err = svc.CreateFromPage(ctx, v, CreatePageTemplateInput{PageID: "p"})
	ptAssertCode(t, err, ErrPageTemplatePlatformOnly)
	_, err = svc.RefreshFromPage(ctx, v, "a")
	ptAssertCode(t, err, ErrPageTemplatePlatformOnly)
	_, err = svc.CopyToPage(ctx, v, "a")
	ptAssertCode(t, err, ErrPageTemplatePlatformOnly)
	_, err = svc.UpdateMeta(ctx, v, "a", UpdatePageTemplateMetaInput{})
	ptAssertCode(t, err, ErrPageTemplatePlatformOnly)

	if repo.calls != 0 || pages.created {
		t.Errorf("repo must not be touched (calls=%d created=%v)", repo.calls, pages.created)
	}
}

func validInput() CreatePageTemplateInput {
	return CreatePageTemplateInput{PageID: "p1", Name: "Klinik", Slug: "klinik", Category: "kesehatan", Description: "d", SortOrder: 10}
}

func grapesPage() *ptPageStub {
	return &ptPageStub{page: domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}}
}

func TestPageTemplateService_CreateFromPage_SanitizesAndLocks(t *testing.T) {
	repo := &ptRepoStub{}
	docs := &ptDocStub{doc: domain.LandingPageDocument{
		HTML: `<section onclick="x()">A</section><div data-zyad-slot="catalog-pricing"></div>`,
		CSS:  `.a{color:red}`,
	}}
	got, err := newPT(repo, docs, grapesPage()).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	if err != nil {
		t.Fatal(err)
	}
	p := repo.createParams
	if strings.Contains(p.HTML, "onclick") {
		t.Errorf("html not sanitized: %s", p.HTML)
	}
	if !p.PlatformOnly || p.Status != domain.PageTemplateStatusDraft || p.Source != domain.PageTemplateSourceAdmin {
		t.Errorf("params: %+v", p)
	}
	if p.SourcePageID == nil || *p.SourcePageID != "p1" || p.CreatedBy != "user-1" || got.ID != "new" {
		t.Errorf("source/creator not set: %+v", p)
	}
}

func TestPageTemplateService_CreateFromPage_Empty(t *testing.T) {
	repo := &ptRepoStub{}
	docs := &ptDocStub{doc: domain.LandingPageDocument{HTML: `<div data-zyad-slot="tenant-nav"></div>`}}
	_, err := newPT(repo, docs, grapesPage()).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	ptAssertCode(t, err, ErrPageTemplateEmpty)
	_, err = newPT(repo, &ptDocStub{err: pgx.ErrNoRows}, grapesPage()).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	ptAssertCode(t, err, ErrPageTemplateEmpty)
	if repo.createParams.Slug != "" {
		t.Error("Create must not be called for empty documents")
	}
}

func TestPageTemplateService_CreateFromPage_SourceNotGrapes(t *testing.T) {
	pages := &ptPageStub{page: domain.LandingPage{ID: "p1", Builder: domain.PageBuilderSections}}
	_, err := newPT(&ptRepoStub{}, &ptDocStub{}, pages).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	ptAssertCode(t, err, ErrPageTemplateNotFound)
	pages = &ptPageStub{findErr: pgx.ErrNoRows}
	_, err = newPT(&ptRepoStub{}, &ptDocStub{}, pages).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	ptAssertCode(t, err, ErrPageTemplateNotFound)
}

func TestPageTemplateService_CreateFromPage_SlugTaken(t *testing.T) {
	repo := &ptRepoStub{createErr: repository.ErrPageTemplateSlugTaken}
	docs := &ptDocStub{doc: domain.LandingPageDocument{HTML: `<section>A</section>`}}
	_, err := newPT(repo, docs, grapesPage()).CreateFromPage(context.Background(), ptViewer(t, true), validInput())
	ptAssertCode(t, err, ErrPageTemplateSlugTaken)
}

func TestPageTemplateService_Refresh_SourceMissing(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{"a": {ID: "a", SourcePageID: nil}}}
	_, err := newPT(repo, &ptDocStub{}, grapesPage()).RefreshFromPage(context.Background(), ptViewer(t, true), "a")
	ptAssertCode(t, err, ErrPageTemplateSourceMissing)

	src := "p1"
	repo = &ptRepoStub{items: map[string]domain.PageTemplate{"a": {ID: "a", SourcePageID: &src}}}
	_, err = newPT(repo, &ptDocStub{}, &ptPageStub{findErr: pgx.ErrNoRows}).RefreshFromPage(context.Background(), ptViewer(t, true), "a")
	ptAssertCode(t, err, ErrPageTemplateSourceMissing)
	if repo.updateCalls != 0 {
		t.Error("Update must not be called when source is missing")
	}
}

func TestPageTemplateService_Refresh_SeedMarksModified(t *testing.T) {
	src := "p1"
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{
		"a": {ID: "a", Source: domain.PageTemplateSourceSeed, SourcePageID: &src},
	}}
	docs := &ptDocStub{doc: domain.LandingPageDocument{HTML: `<section>baru</section>`, CSS: ``}}
	if _, err := newPT(repo, docs, grapesPage()).RefreshFromPage(context.Background(), ptViewer(t, true), "a"); err != nil {
		t.Fatal(err)
	}
	if repo.updateParams.SeedModified == nil || !*repo.updateParams.SeedModified {
		t.Errorf("SeedModified must be true: %+v", repo.updateParams)
	}
	if repo.updateParams.HTML == nil || *repo.updateParams.HTML != `<section>baru</section>` {
		t.Errorf("html not updated: %+v", repo.updateParams)
	}
}

func TestPageTemplateService_UpdateMeta_CannotUnlockPlatformOnly(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{
		"a": {ID: "a", Slug: "a", Name: "n", Category: "dasar", HTML: `<div data-zyad-slot="catalog-pricing"></div>`, PlatformOnly: true},
	}}
	off := false
	_, err := newPT(repo, &ptDocStub{}, &ptPageStub{}).UpdateMeta(context.Background(), ptViewer(t, true), "a",
		UpdatePageTemplateMetaInput{PlatformOnly: &off})
	if err != nil {
		t.Fatal(err)
	}
	if repo.updateParams.PlatformOnly == nil || !*repo.updateParams.PlatformOnly {
		t.Errorf("platform_only must stay true: %+v", repo.updateParams)
	}
}

func TestPageTemplateService_UpdateMeta_InvalidStatus(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{"a": {ID: "a", Name: "n", Slug: "a", Category: "dasar"}}}
	bad := domain.PageTemplateStatus("deleted")
	_, err := newPT(repo, &ptDocStub{}, &ptPageStub{}).UpdateMeta(context.Background(), ptViewer(t, true), "a",
		UpdatePageTemplateMetaInput{Status: &bad})
	if err == nil || repo.updateCalls != 0 {
		t.Fatalf("invalid status must be rejected (err=%v)", err)
	}
}

func TestPageTemplateService_CopyToPage(t *testing.T) {
	repo := &ptRepoStub{items: map[string]domain.PageTemplate{
		"a": {ID: "a", Slug: "klinik", Name: "Klinik", HTML: "<section>x</section>", CSS: ".x{}"},
	}}
	docs := &ptDocStub{}
	pages := &ptPageStub{takenSlugs: map[string]bool{"template-klinik": true}}
	page, err := newPT(repo, docs, pages).CopyToPage(context.Background(), ptViewer(t, true), "a")
	if err != nil {
		t.Fatal(err)
	}
	p := pages.createParams
	if p.Builder != domain.PageBuilderGrapesJS || p.Status != domain.PageStatusDraft ||
		p.Visibility != domain.PageVisibilityPrivate || p.IsTemplate || p.Slug != "template-klinik-2" {
		t.Errorf("page params: %+v", p)
	}
	if docs.up.HTML != "<section>x</section>" || docs.up.LandingPageID != "page-new" {
		t.Errorf("document upsert: %+v", docs.up)
	}
	if repo.updateParams.SourcePageID == nil || *repo.updateParams.SourcePageID != "page-new" || page.ID != "page-new" {
		t.Errorf("source link: %+v", repo.updateParams)
	}
}
