package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/platform/database"
)

type categoryRepository struct{ db *database.Pool }

func NewCategoryRepository(db *database.Pool) CategoryRepository { return &categoryRepository{db: db} }

const categoryColumns = "id, organization_id, name, position, created_at, updated_at"

func scanCategory(row pgx.Row) (domain.Category, error) {
	var c domain.Category
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Position, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *categoryRepository) Create(ctx context.Context, scope coretenant.Scope, name string, position int, createdBy string) (domain.Category, error) {
	var out domain.Category
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanCategory(tx.QueryRow(ctx, `INSERT INTO catalog_product_categories (organization_id, name, position, created_by)
			VALUES ($1, $2, $3, $4) RETURNING `+categoryColumns, scope.OrganizationID(), strings.TrimSpace(name), position, nullable(createdBy)))
		return err
	})
	if isUniqueViolation(err, "idx_catalog_categories_org_name_unique") {
		return domain.Category{}, ErrCategoryNameExists
	}
	return out, err
}

func (r *categoryRepository) List(ctx context.Context, scope coretenant.Scope) ([]domain.Category, error) {
	out := []domain.Category{}
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+categoryColumns+` FROM catalog_product_categories
			WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY position, lower(name)`, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCategory(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (r *categoryRepository) Update(ctx context.Context, scope coretenant.Scope, id string, name *string, position *int, updatedBy string) (domain.Category, error) {
	sets := []string{"updated_at = NOW()"}
	var args []any
	if name != nil {
		args = append(args, strings.TrimSpace(*name))
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if position != nil {
		args = append(args, *position)
		sets = append(sets, fmt.Sprintf("position = $%d", len(args)))
	}
	if updatedBy != "" {
		args = append(args, updatedBy)
		sets = append(sets, fmt.Sprintf("updated_by = $%d", len(args)))
	}
	args = append(args, id, scope.OrganizationID())
	q := "UPDATE catalog_product_categories SET " + strings.Join(sets, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) + categoryColumns
	var out domain.Category
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanCategory(tx.QueryRow(ctx, q, args...))
		return err
	})
	if isUniqueViolation(err, "idx_catalog_categories_org_name_unique") {
		return domain.Category{}, ErrCategoryNameExists
	}
	return out, err
}

// Delete soft-delete kategori dan mengosongkan category_id produknya
// (FK ON DELETE SET NULL tidak terpicu oleh soft delete).
func (r *categoryRepository) Delete(ctx context.Context, scope coretenant.Scope, id, deletedBy string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE catalog_product_categories SET deleted_at = NOW(), updated_by = $1
			WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL`, nullable(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		_, err = tx.Exec(ctx, `UPDATE catalog_products SET category_id = NULL, updated_at = NOW()
			WHERE organization_id = $1 AND category_id = $2`, scope.OrganizationID(), id)
		return err
	})
}
