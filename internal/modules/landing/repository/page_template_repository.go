package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

// seedLockKey adalah kunci advisory lock untuk seeder template.
const seedLockKey = 7310015

const pageTemplateColumns = `id::text, slug, name, description, category, sort_order, status,
	platform_only, source, seed_version, seed_modified, source_page_id::text,
	COALESCE(created_by::text, ''), COALESCE(updated_by::text, ''), created_at, updated_at`

type platformPageTemplateStore struct {
	db *database.Pool
}

func NewPageTemplateRepository(db *database.Pool) PlatformPageTemplateRepository {
	return &platformPageTemplateStore{db: db}
}

func scanPageTemplate(row pgx.Row, withBody bool) (domain.PageTemplate, error) {
	var t domain.PageTemplate
	var status, source string
	dest := []any{
		&t.ID, &t.Slug, &t.Name, &t.Description, &t.Category, &t.SortOrder, &status,
		&t.PlatformOnly, &source, &t.SeedVersion, &t.SeedModified, &t.SourcePageID,
		&t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt,
	}
	if withBody {
		dest = append(dest, &t.HTML, &t.CSS)
	}
	if err := row.Scan(dest...); err != nil {
		return domain.PageTemplate{}, err
	}
	t.Status = domain.PageTemplateStatus(status)
	t.Source = domain.PageTemplateSource(source)
	return t, nil
}

func (r *platformPageTemplateStore) List(ctx context.Context, f PageTemplateListFilter) ([]domain.PageTemplate, error) {
	var where []string
	var args []any
	if len(f.Statuses) > 0 {
		statuses := make([]string, len(f.Statuses))
		for i, s := range f.Statuses {
			statuses[i] = string(s)
		}
		args = append(args, statuses)
		where = append(where, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if !f.IncludePlatformOnly {
		where = append(where, "platform_only = false")
	}
	query := "SELECT " + pageTemplateColumns + " FROM landing_page_templates"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY sort_order, name"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.PageTemplate{}
	for rows.Next() {
		t, err := scanPageTemplate(rows, false)
		if err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *platformPageTemplateStore) findOne(ctx context.Context, column, value string) (domain.PageTemplate, error) {
	query := "SELECT " + pageTemplateColumns + ", html, css FROM landing_page_templates WHERE " + column + " = $1"
	return scanPageTemplate(r.db.QueryRow(ctx, query, value), true)
}

func (r *platformPageTemplateStore) FindByID(ctx context.Context, id string) (domain.PageTemplate, error) {
	// Dibandingkan sebagai text agar id yang bukan uuid menjadi "tidak ada", bukan error SQL.
	// Tabel kecil (puluhan baris), jadi tidak memakai index PK.
	query := "SELECT " + pageTemplateColumns + ", html, css FROM landing_page_templates WHERE id::text = $1"
	return scanPageTemplate(r.db.QueryRow(ctx, query, id), true)
}

func (r *platformPageTemplateStore) FindBySlug(ctx context.Context, slug string) (domain.PageTemplate, error) {
	return r.findOne(ctx, "slug", slug)
}

func (r *platformPageTemplateStore) Create(ctx context.Context, p CreatePageTemplateParams) (domain.PageTemplate, error) {
	var createdBy *string
	if p.CreatedBy != "" {
		createdBy = &p.CreatedBy
	}
	query := `
		INSERT INTO landing_page_templates (
			slug, name, description, category, sort_order, status, platform_only,
			html, css, source, seed_version, source_page_id, created_by, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
		RETURNING ` + pageTemplateColumns + `, html, css`
	t, err := scanPageTemplate(r.db.QueryRow(ctx, query,
		p.Slug, p.Name, p.Description, p.Category, p.SortOrder, string(p.Status), p.PlatformOnly,
		p.HTML, p.CSS, string(p.Source), p.SeedVersion, p.SourcePageID, createdBy,
	), true)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.PageTemplate{}, ErrPageTemplateSlugTaken
		}
		return domain.PageTemplate{}, err
	}
	return t, nil
}

func (r *platformPageTemplateStore) Update(ctx context.Context, id string, p UpdatePageTemplateParams) (domain.PageTemplate, error) {
	sets := []string{"updated_at = now()"}
	args := []any{id}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.SortOrder != nil {
		add("sort_order", *p.SortOrder)
	}
	if p.Status != nil {
		add("status", string(*p.Status))
	}
	if p.PlatformOnly != nil {
		add("platform_only", *p.PlatformOnly)
	}
	if p.HTML != nil {
		add("html", *p.HTML)
	}
	if p.CSS != nil {
		add("css", *p.CSS)
	}
	if p.SeedVersion != nil {
		add("seed_version", *p.SeedVersion)
	}
	if p.SeedModified != nil {
		add("seed_modified", *p.SeedModified)
	}
	if p.SourcePageID != nil {
		add("source_page_id", *p.SourcePageID)
	}
	if p.UpdatedBy != "" {
		add("updated_by", p.UpdatedBy)
	}
	query := "UPDATE landing_page_templates SET " + strings.Join(sets, ", ") +
		" WHERE id::text = $1 RETURNING " + pageTemplateColumns + ", html, css"
	return scanPageTemplate(r.db.QueryRow(ctx, query, args...), true)
}

func (r *platformPageTemplateStore) WithSeedLock(ctx context.Context, fn func(ctx context.Context) error) error {
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return err
	}
	released := false
	defer func() {
		if !released {
			conn.Release()
		}
	}()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", seedLockKey); err != nil {
		return err
	}
	defer func() {
		// Context bisa sudah dibatalkan saat fn selesai; unlock tetap dicoba.
		if _, err := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", seedLockKey); err != nil {
			// Lock level session tidak boleh kembali ke pool; tutup koneksinya.
			released = true
			_ = conn.Hijack().Close(context.WithoutCancel(ctx))
		}
	}()
	return fn(ctx)
}
