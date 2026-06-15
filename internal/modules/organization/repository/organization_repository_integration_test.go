//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestOrganizationRepositoryLifecycleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := NewOrganizationRepository(db)
	ctx := context.Background()

	slug := strings.ReplaceAll("org-"+testutil.UniqueCode("repo"), ".", "-")
	organization, err := repo.Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "  " + slug + "  ",
		Name:          "Repository Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
		Metadata:      map[string]any{"source": "integration"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	if organization.Slug != slug {
		t.Fatalf("Create() slug = %q, want %q", organization.Slug, slug)
	}

	found, err := repo.FindBySlug(ctx, "  "+slug+"  ")
	if err != nil {
		t.Fatalf("FindBySlug() error = %v", err)
	}
	if found.ID != organization.ID {
		t.Fatalf("FindBySlug() ID = %q, want %q", found.ID, organization.ID)
	}

	found, err = repo.FindByID(ctx, organization.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.Slug != slug {
		t.Fatalf("FindByID() slug = %q, want %q", found.Slug, slug)
	}

	_, err = repo.Create(ctx, CreateOrganizationParams{
		Type: coretenant.OrganizationTypeCustomer,
		Slug: strings.ToUpper(slug),
		Name: "Duplicate Repository Test",
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "idx_organizations_slug_active_unique" {
		t.Fatalf("Create() duplicate error = %v, want slug unique violation", err)
	}

	list, total, err := repo.List(ctx, OrganizationListFilter{
		Search: slug,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("List() total = %d, length = %d, want 1", total, len(list))
	}

	name := "Updated Repository Test"
	updated, err := repo.Update(ctx, organization.ID, UpdateOrganizationParams{
		Name: &name,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != name {
		t.Fatalf("Update() name = %q, want %q", updated.Name, name)
	}

	suspended, err := repo.UpdateStatus(ctx, organization.ID, coretenant.OrganizationStatusSuspended)
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if suspended.Status != coretenant.OrganizationStatusSuspended {
		t.Fatalf("UpdateStatus() status = %q", suspended.Status)
	}

	if err := repo.Archive(ctx, organization.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if _, err := repo.FindByID(ctx, organization.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("FindByID() archived error = %v, want pgx.ErrNoRows", err)
	}

	list, total, err = repo.List(ctx, OrganizationListFilter{Search: slug})
	if err != nil {
		t.Fatalf("List() after archive error = %v", err)
	}
	if total != 0 || len(list) != 0 {
		t.Fatalf("List() after archive total = %d, length = %d, want 0", total, len(list))
	}
}
