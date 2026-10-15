package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/templates"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// htmlSignature menghitung token "<tag>", "<tag> attr=nilai" dan teks ber-trim.
func htmlSignature(t *testing.T, src string) map[string]int {
	t.Helper()
	sig := map[string]int{}
	z := html.NewTokenizer(strings.NewReader(src))
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				t.Fatalf("tokenize: %v", z.Err())
			}
			return sig
		case html.TextToken:
			if txt := strings.TrimSpace(string(z.Text())); txt != "" {
				sig["text:"+txt]++
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			sig["<"+tok.Data+">"]++
			for _, a := range tok.Attr {
				sig["<"+tok.Data+"> "+a.Key+"="+a.Val]++
			}
		}
	}
}

func TestSeeds_SanitizerRemovesNothing(t *testing.T) {
	seeds, err := templates.LoadSeeds()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range seeds {
		before := htmlSignature(t, s.HTML)
		after := htmlSignature(t, SanitizeGrapesHTML(s.HTML))
		for k, n := range before {
			if after[k] < n {
				t.Errorf("%s: sanitizer menghapus %q (%d -> %d)", s.Slug, k, n, after[k])
			}
		}
		if got := SanitizeGrapesCSS(s.CSS); got != s.CSS {
			t.Errorf("%s: sanitizer mengubah CSS (len %d -> %d)", s.Slug, len(s.CSS), len(got))
		}
	}
}

func TestDecideSeedAction(t *testing.T) {
	v1, v2 := 1, 2
	s := templates.Seed{Version: 2}
	cases := []struct {
		name string
		in   *domain.PageTemplate
		want seedAction
	}{
		{"belum ada", nil, seedInsert},
		{"milik admin", &domain.PageTemplate{Source: domain.PageTemplateSourceAdmin}, seedSkip},
		{"diedit admin", &domain.PageTemplate{Source: domain.PageTemplateSourceSeed, SeedModified: true, SeedVersion: &v1}, seedSkip},
		{"versi naik", &domain.PageTemplate{Source: domain.PageTemplateSourceSeed, SeedVersion: &v1}, seedUpdate},
		{"versi naik, diarsipkan", &domain.PageTemplate{Source: domain.PageTemplateSourceSeed, SeedVersion: &v1, Status: domain.PageTemplateStatusArchived}, seedUpdate},
		{"versi sama", &domain.PageTemplate{Source: domain.PageTemplateSourceSeed, SeedVersion: &v2}, seedSkip},
		{"seed_version null", &domain.PageTemplate{Source: domain.PageTemplateSourceSeed}, seedUpdate},
	}
	for _, c := range cases {
		if got := decideSeedAction(c.in, s); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

type seederRepoStub struct {
	ptRepoStub
	existing  map[string]domain.PageTemplate
	creates   []repository.CreatePageTemplateParams
	updates   []repository.UpdatePageTemplateParams
	createErr error
	lockCalls int
}

func (s *seederRepoStub) FindBySlug(_ context.Context, slug string) (domain.PageTemplate, error) {
	if t, ok := s.existing[slug]; ok {
		return t, nil
	}
	return domain.PageTemplate{}, pgx.ErrNoRows
}
func (s *seederRepoStub) Create(_ context.Context, p repository.CreatePageTemplateParams) (domain.PageTemplate, error) {
	if s.createErr != nil {
		return domain.PageTemplate{}, s.createErr
	}
	s.creates = append(s.creates, p)
	return domain.PageTemplate{}, nil
}
func (s *seederRepoStub) Update(_ context.Context, _ string, p repository.UpdatePageTemplateParams) (domain.PageTemplate, error) {
	s.updates = append(s.updates, p)
	return domain.PageTemplate{}, nil
}
func (s *seederRepoStub) WithSeedLock(ctx context.Context, fn func(context.Context) error) error {
	s.lockCalls++
	return fn(ctx)
}

func realSeeds(t *testing.T) []templates.Seed {
	t.Helper()
	seeds, err := templates.LoadSeeds()
	if err != nil {
		t.Fatal(err)
	}
	return seeds
}

func TestSeedPageTemplates_InsertsAll(t *testing.T) {
	repo := &seederRepoStub{}
	seeds := realSeeds(t)
	if err := SeedPageTemplates(context.Background(), repo, seeds, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(repo.creates) != len(seeds) || repo.lockCalls != 1 {
		t.Fatalf("creates=%d lock=%d", len(repo.creates), repo.lockCalls)
	}
	for _, c := range repo.creates {
		if c.Status != domain.PageTemplateStatusPublished || c.Source != domain.PageTemplateSourceSeed || c.SeedVersion == nil {
			t.Errorf("%s: %+v", c.Slug, c)
		}
	}
}

func TestSeedPageTemplates_SameVersionWritesNothing(t *testing.T) {
	seeds := realSeeds(t)
	repo := &seederRepoStub{existing: map[string]domain.PageTemplate{}}
	for _, s := range seeds {
		v := s.Version
		repo.existing[s.Slug] = domain.PageTemplate{ID: s.Slug, Source: domain.PageTemplateSourceSeed, SeedVersion: &v}
	}
	if err := SeedPageTemplates(context.Background(), repo, seeds, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(repo.creates)+len(repo.updates) != 0 {
		t.Errorf("expected no writes, got %d creates %d updates", len(repo.creates), len(repo.updates))
	}
}

func TestSeedPageTemplates_SlugRaceIsNotAnError(t *testing.T) {
	repo := &seederRepoStub{createErr: repository.ErrPageTemplateSlugTaken}
	if err := SeedPageTemplates(context.Background(), repo, realSeeds(t), discardLogger()); err != nil {
		t.Fatalf("slug conflict must be treated as already present: %v", err)
	}
}

func TestSeedPageTemplates_UpdateKeepsStatus(t *testing.T) {
	seeds := realSeeds(t)[:1]
	old := 0
	repo := &seederRepoStub{existing: map[string]domain.PageTemplate{
		seeds[0].Slug: {ID: "x", Source: domain.PageTemplateSourceSeed, SeedVersion: &old, Status: domain.PageTemplateStatusArchived},
	}}
	if err := SeedPageTemplates(context.Background(), repo, seeds, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("updates=%d", len(repo.updates))
	}
	u := repo.updates[0]
	if u.Status != nil || u.HTML == nil || u.SeedVersion == nil || *u.SeedVersion != seeds[0].Version {
		t.Errorf("update params: %+v", u)
	}
	if u.SeedModified != nil {
		t.Error("seeder must not touch seed_modified")
	}
}

func TestSeedPageTemplates_LockFailureReturnsError(t *testing.T) {
	repo := &failingLockRepo{}
	if err := SeedPageTemplates(context.Background(), repo, realSeeds(t), discardLogger()); err == nil {
		t.Fatal("expected error when lock fails")
	}
}

type failingLockRepo struct{ seederRepoStub }

func (f *failingLockRepo) WithSeedLock(context.Context, func(context.Context) error) error {
	return errors.New("lock failed")
}
