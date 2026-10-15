package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/templates"
)

type seedAction int

const (
	seedSkip seedAction = iota
	seedInsert
	seedUpdate
)

// decideSeedAction menerapkan aturan spec §5.5.
func decideSeedAction(existing *domain.PageTemplate, s templates.Seed) seedAction {
	switch {
	case existing == nil:
		return seedInsert
	case existing.Source == domain.PageTemplateSourceAdmin:
		return seedSkip
	case existing.SeedModified:
		return seedSkip
	case existing.SeedVersion == nil || *existing.SeedVersion < s.Version:
		return seedUpdate
	}
	return seedSkip
}

// SeedPageTemplates memasukkan file seed ke DB secara idempoten. Seluruh run
// berada dalam advisory lock supaya beberapa instance yang start bersamaan
// tidak saling menimpa. Error per-seed dicatat; hanya kegagalan lock yang
// dikembalikan.
func SeedPageTemplates(ctx context.Context, repo repository.PlatformPageTemplateRepository, seeds []templates.Seed, log *slog.Logger) error {
	return repo.WithSeedLock(ctx, func(ctx context.Context) error {
		for _, s := range seeds {
			if err := seedOne(ctx, repo, s, log); err != nil {
				log.Error("seed page template gagal", "slug", s.Slug, "error", err)
			}
		}
		return nil
	})
}

func seedOne(ctx context.Context, repo repository.PlatformPageTemplateRepository, s templates.Seed, log *slog.Logger) error {
	var existing *domain.PageTemplate
	found, err := repo.FindBySlug(ctx, s.Slug)
	switch {
	case err == nil:
		existing = &found
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	html, css := SanitizeGrapesHTML(s.HTML), SanitizeGrapesCSS(s.CSS)
	platformOnly := LockPlatformOnly(html, s.PlatformOnly)
	version := s.Version

	switch decideSeedAction(existing, s) {
	case seedInsert:
		_, err := repo.Create(ctx, repository.CreatePageTemplateParams{
			Slug: s.Slug, Name: s.Name, Description: s.Description, Category: s.Category,
			SortOrder: s.SortOrder, Status: domain.PageTemplateStatusPublished,
			PlatformOnly: platformOnly, HTML: html, CSS: css,
			Source: domain.PageTemplateSourceSeed, SeedVersion: &version,
		})
		if errors.Is(err, repository.ErrPageTemplateSlugTaken) {
			return nil // instance lain menang balapan
		}
		return err
	case seedUpdate:
		_, err := repo.Update(ctx, existing.ID, repository.UpdatePageTemplateParams{
			Name: &s.Name, Description: &s.Description, Category: &s.Category, SortOrder: &s.SortOrder,
			PlatformOnly: &platformOnly, HTML: &html, CSS: &css, SeedVersion: &version,
		})
		return err
	default:
		if existing != nil && existing.Source == domain.PageTemplateSourceAdmin {
			log.Warn("page template seed slug dipakai template admin", "slug", s.Slug)
		}
	}
	return nil
}
