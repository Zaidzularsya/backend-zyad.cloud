package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

type productRepository struct{ db *database.Pool }

func NewProductRepository(db *database.Pool) ProductRepository { return &productRepository{db: db} }

func withTx(ctx context.Context, db *database.Pool, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullable(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}

// isUniqueViolation memetakan index unik ke sentinel error.
func isUniqueViolation(err error, index string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == index
}

const productSelect = `
	SELECT p.id, p.organization_id, p.category_id, COALESCE(c.name, ''), p.sku, p.name, p.description,
		p.unit, p.base_price::text, p.tax_percent::text, p.currency, p.charge_type, p.billing_frequency, p.payment_timing, p.is_active,
		p.is_public, COALESCE(p.listing_code, ''), p.listing_order, COALESCE(c.position, 0),
		p.created_by, p.updated_by, p.created_at, p.updated_at, p.deleted_at
	FROM catalog_products p
	LEFT JOIN catalog_product_categories c
		ON c.id = p.category_id AND c.organization_id = p.organization_id AND c.deleted_at IS NULL`

func scanProduct(row pgx.Row) (domain.Product, error) {
	var p domain.Product
	var sku, description, createdBy, updatedBy, frequency *string
	err := row.Scan(&p.ID, &p.OrganizationID, &p.CategoryID, &p.CategoryName, &sku, &p.Name, &description,
		&p.Unit, &p.BasePrice, &p.TaxPercent, &p.Currency, &p.Pricing.ChargeType, &frequency, &p.Pricing.PaymentTiming, &p.IsActive,
		&p.IsPublic, &p.ListingCode, &p.ListingOrder, &p.CategoryPosition,
		&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if frequency != nil {
		p.Pricing.Frequency = pricing.Frequency(*frequency)
	}
	if sku != nil {
		p.SKU = *sku
	}
	if description != nil {
		p.Description = *description
	}
	if createdBy != nil {
		p.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		p.UpdatedBy = *updatedBy
	}
	return p, err
}

func (r *productRepository) getTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, id string) (domain.Product, error) {
	p, err := scanProduct(tx.QueryRow(ctx, productSelect+` WHERE p.id = $1 AND p.organization_id = $2 AND p.deleted_at IS NULL`, id, scope.OrganizationID()))
	if err != nil {
		return p, err
	}
	list := []domain.Product{p}
	if err := attachFeatures(ctx, tx, scope, list); err != nil {
		return p, err
	}
	return list[0], nil
}

// attachFeatures mengisi Features semua produk dengan satu query (tanpa N+1).
func attachFeatures(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, products []domain.Product) error {
	if len(products) == 0 {
		return nil
	}
	ids := make([]string, len(products))
	index := make(map[string]int, len(products))
	for i := range products {
		ids[i] = products[i].ID
		index[products[i].ID] = i
		products[i].Features = []domain.ProductFeature{}
	}
	rows, err := tx.Query(ctx, `
		SELECT product_id, feature_key, value, COALESCE(display_label, ''), position
		FROM catalog_product_features
		WHERE organization_id = $1 AND product_id = ANY($2::uuid[])
		ORDER BY position, feature_key`, scope.OrganizationID(), ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var productID string
		var f domain.ProductFeature
		if err := rows.Scan(&productID, &f.FeatureKey, &f.Value, &f.DisplayLabel, &f.Position); err != nil {
			return err
		}
		i := index[productID]
		products[i].Features = append(products[i].Features, f)
	}
	return rows.Err()
}

// replaceFeatures menimpa seluruh fitur produk (delete lalu insert).
func replaceFeatures(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, productID string, features []FeatureValue) error {
	if _, err := tx.Exec(ctx, `DELETE FROM catalog_product_features WHERE organization_id = $1 AND product_id = $2`, scope.OrganizationID(), productID); err != nil {
		return err
	}
	for _, f := range features {
		if _, err := tx.Exec(ctx, `
			INSERT INTO catalog_product_features (organization_id, product_id, feature_key, value, display_label, position)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			scope.OrganizationID(), productID, f.FeatureKey, []byte(f.Value), nullable(f.DisplayLabel), f.Position); err != nil {
			return err
		}
	}
	return nil
}

func mapListingErr(err error) error {
	if isUniqueViolation(err, "idx_catalog_products_listing_unique") {
		return ErrListingExists
	}
	return err
}

func (r *productRepository) Create(ctx context.Context, scope coretenant.Scope, in CreateProductParams) (domain.Product, error) {
	var out domain.Product
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var id string
		currency := in.Currency
		if currency == "" {
			currency = "IDR"
		}
		// Service sudah menormalisasi; ini menjaga pemanggil langsung tanpa atribut harga.
		if in.Pricing.ChargeType == "" {
			in.Pricing.ChargeType = pricing.OneTime
		}
		if in.Pricing.PaymentTiming == "" {
			in.Pricing.PaymentTiming = pricing.Prepaid
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO catalog_products (organization_id, category_id, sku, name, description, unit,
				base_price, tax_percent, currency, charge_type, billing_frequency, payment_timing, is_active, created_by,
				is_public, listing_code, listing_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING id`,
			scope.OrganizationID(), nullable(in.CategoryID), nullable(in.SKU), strings.TrimSpace(in.Name),
			nullable(in.Description), strings.TrimSpace(in.Unit), in.BasePrice, in.TaxPercent, currency,
			string(in.Pricing.ChargeType), nullable(string(in.Pricing.Frequency)), string(in.Pricing.PaymentTiming), in.IsActive, nullable(in.CreatedBy),
			in.Listing.IsPublic, nullable(in.Listing.ListingCode), in.Listing.ListingOrder).Scan(&id)
		if err != nil {
			return err
		}
		if err := replaceFeatures(ctx, tx, scope, id, in.Features); err != nil {
			return err
		}
		out, err = r.getTx(ctx, tx, scope, id)
		return err
	})
	if isUniqueViolation(err, "idx_catalog_products_org_sku_unique") {
		return domain.Product{}, ErrSKUExists
	}
	return out, mapListingErr(err)
}

func (r *productRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Product, error) {
	var out domain.Product
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = r.getTx(ctx, tx, scope, id)
		return err
	})
	return out, err
}

func (r *productRepository) FindBySKU(ctx context.Context, scope coretenant.Scope, sku string) (domain.Product, error) {
	var out domain.Product
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		p, err := scanProduct(tx.QueryRow(ctx, productSelect+` WHERE p.sku = $1 AND p.organization_id = $2 AND p.is_active AND p.deleted_at IS NULL`,
			sku, scope.OrganizationID()))
		if err != nil {
			return err
		}
		list := []domain.Product{p}
		if err := attachFeatures(ctx, tx, scope, list); err != nil {
			return err
		}
		out = list[0]
		return nil
	})
	return out, err
}

func (r *productRepository) FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]domain.Product, error) {
	out := make(map[string]domain.Product, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, productSelect+` WHERE p.organization_id = $1 AND p.id = ANY($2::uuid[]) AND p.deleted_at IS NULL`, scope.OrganizationID(), ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanProduct(rows)
			if err != nil {
				return err
			}
			out[p.ID] = p
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachMap(ctx, tx, scope, out)
	})
	return out, err
}

func attachMap(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, m map[string]domain.Product) error {
	list := make([]domain.Product, 0, len(m))
	for _, p := range m {
		list = append(list, p)
	}
	if err := attachFeatures(ctx, tx, scope, list); err != nil {
		return err
	}
	for _, p := range list {
		m[p.ID] = p
	}
	return nil
}

func (r *productRepository) ListPublic(ctx context.Context, scope coretenant.Scope) ([]domain.Product, error) {
	list := []domain.Product{}
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, productSelect+` WHERE p.organization_id = $1 AND p.is_public AND p.is_active AND p.deleted_at IS NULL
			ORDER BY COALESCE(c.position, 0), lower(COALESCE(c.name, '')), p.listing_order, p.listing_code, p.id`, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanProduct(rows)
			if err != nil {
				return err
			}
			list = append(list, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachFeatures(ctx, tx, scope, list)
	})
	return list, err
}

func (r *productRepository) List(ctx context.Context, scope coretenant.Scope, f ProductListFilter) ([]domain.Product, int64, error) {
	where := []string{"p.organization_id = $1", "p.deleted_at IS NULL"}
	args := []any{scope.OrganizationID()}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+strings.ToLower(s)+"%")
		where = append(where, fmt.Sprintf("(lower(p.name) LIKE $%d OR lower(COALESCE(p.sku, '')) LIKE $%d)", len(args), len(args)))
	}
	if f.CategoryID != "" {
		args = append(args, f.CategoryID)
		where = append(where, fmt.Sprintf("p.category_id = $%d", len(args)))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		where = append(where, fmt.Sprintf("p.is_active = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")
	var list []domain.Product
	var total int64
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM catalog_products p WHERE "+cond, args...).Scan(&total); err != nil {
			return err
		}
		q := productSelect + " WHERE " + cond + " ORDER BY lower(p.name), p.id"
		qa := append([]any{}, args...)
		if f.Limit > 0 {
			qa = append(qa, f.Limit)
			q += fmt.Sprintf(" LIMIT $%d", len(qa))
		}
		if f.Offset > 0 {
			qa = append(qa, f.Offset)
			q += fmt.Sprintf(" OFFSET $%d", len(qa))
		}
		rows, err := tx.Query(ctx, q, qa...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanProduct(rows)
			if err != nil {
				return err
			}
			list = append(list, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachFeatures(ctx, tx, scope, list)
	})
	if list == nil {
		list = []domain.Product{}
	}
	return list, total, err
}

func (r *productRepository) Update(ctx context.Context, scope coretenant.Scope, id string, in UpdateProductParams) (domain.Product, error) {
	sets := []string{"updated_at = NOW()"}
	var args []any
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if in.CategoryID != nil {
		add("category_id", nullable(*in.CategoryID))
	}
	if in.SKU != nil {
		add("sku", nullable(*in.SKU))
	}
	if in.Name != nil {
		add("name", strings.TrimSpace(*in.Name))
	}
	if in.Description != nil {
		add("description", nullable(*in.Description))
	}
	if in.Unit != nil {
		add("unit", strings.TrimSpace(*in.Unit))
	}
	if in.BasePrice != nil {
		add("base_price", *in.BasePrice)
	}
	if in.TaxPercent != nil {
		add("tax_percent", *in.TaxPercent)
	}
	if in.Pricing != nil {
		add("charge_type", string(in.Pricing.ChargeType))
		add("billing_frequency", nullable(string(in.Pricing.Frequency)))
		add("payment_timing", string(in.Pricing.PaymentTiming))
	}
	if in.IsActive != nil {
		add("is_active", *in.IsActive)
	}
	if in.Listing != nil {
		add("is_public", in.Listing.IsPublic)
		add("listing_code", nullable(in.Listing.ListingCode))
		add("listing_order", in.Listing.ListingOrder)
	}
	if in.UpdatedBy != "" {
		add("updated_by", in.UpdatedBy)
	}
	args = append(args, id, scope.OrganizationID())
	q := "UPDATE catalog_products SET " + strings.Join(sets, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING id", len(args)-1, len(args))
	var out domain.Product
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var got string
		if err := tx.QueryRow(ctx, q, args...).Scan(&got); err != nil {
			return err
		}
		if in.Features != nil {
			if err := replaceFeatures(ctx, tx, scope, got, *in.Features); err != nil {
				return err
			}
		}
		var err error
		out, err = r.getTx(ctx, tx, scope, got)
		return err
	})
	if isUniqueViolation(err, "idx_catalog_products_org_sku_unique") {
		return domain.Product{}, ErrSKUExists
	}
	return out, mapListingErr(err)
}

func (r *productRepository) Delete(ctx context.Context, scope coretenant.Scope, id, deletedBy string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE catalog_products SET deleted_at = NOW(), updated_by = $1
			WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL`, nullable(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
