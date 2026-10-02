package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/quotationpdf"
	"zyad.cloud/internal/platform/database"
)

type QuotationIssuerRepository interface {
	Find(ctx context.Context, scope coretenant.Scope) (quotationpdf.Issuer, error)
}

type quotationIssuerRepository struct{ db *database.Pool }

func NewQuotationIssuerRepository(db *database.Pool) QuotationIssuerRepository {
	return &quotationIssuerRepository{db: db}
}

// Find: nama dari branding default (landing_brandings tanpa page) bila ada,
// selain itu nama organisasi; kontak dari branding.contact {email, phone, address}.
func (r *quotationIssuerRepository) Find(ctx context.Context, scope coretenant.Scope) (quotationpdf.Issuer, error) {
	var issuer quotationpdf.Issuer
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var orgName string
		var brandName *string
		var contact []byte
		err := tx.QueryRow(ctx, `
			SELECT o.name, b.company_name, COALESCE(b.contact, '{}'::jsonb)
			FROM organizations o
			LEFT JOIN landing_brandings b ON b.organization_id = o.id AND b.landing_page_id IS NULL
			WHERE o.id = $1`, scope.OrganizationID()).Scan(&orgName, &brandName, &contact)
		if err != nil {
			return err
		}
		issuer.Name = orgName
		if brandName != nil && *brandName != "" {
			issuer.Name = *brandName
		}
		var c struct{ Email, Phone, Address string }
		_ = json.Unmarshal(contact, &c)
		issuer.Email, issuer.Phone, issuer.Address = c.Email, c.Phone, c.Address
		return nil
	})
	return issuer, err
}
