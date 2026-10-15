package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type quotationResponseRepository struct{ db *database.Pool }

func NewQuotationResponseRepository(db *database.Pool) QuotationResponseRepository {
	return &quotationResponseRepository{db: db}
}

const quotationResponseColumns = `
	id::text, quotation_id::text, COALESCE(link_id::text, ''), action, revision_categories,
	COALESCE(note, ''), responder_name, COALESCE(host(ip), ''), COALESCE(user_agent, ''), created_at`

func scanQuotationResponse(row pgx.Row) (domain.QuotationResponse, error) {
	var r domain.QuotationResponse
	var action string
	var categories []string
	if err := row.Scan(&r.ID, &r.QuotationID, &r.LinkID, &action, &categories,
		&r.Note, &r.ResponderName, &r.IP, &r.UserAgent, &r.CreatedAt); err != nil {
		return domain.QuotationResponse{}, err
	}
	r.Action = domain.QuotationResponseAction(action)
	for _, c := range categories {
		r.Categories = append(r.Categories, domain.RevisionCategory(c))
	}
	return r, nil
}

func (r *quotationResponseRepository) Respond(ctx context.Context, scope coretenant.Scope, p RecordResponseParams) (domain.Quotation, domain.QuotationResponse, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, domain.QuotationResponse{}, coretenant.ErrInvalidScope
	}
	newStatus := domain.QuotationStatusRevisionRequested
	if p.Action == domain.QuotationResponseApproved {
		newStatus = domain.QuotationStatusApproved
	}
	categories := make([]string, 0, len(p.Categories))
	for _, c := range p.Categories {
		categories = append(categories, string(c))
	}

	var quotation domain.Quotation
	var response domain.QuotationResponse
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		// Kondisional pada status = 'sent': respons ganda (dua klik / dua tab)
		// kalah di sini tanpa menulis apa pun.
		var id string
		err := tx.QueryRow(ctx, `
			UPDATE crm_quotations
			SET status = $3::text,
				approved_at = CASE WHEN $3::text = 'approved' THEN NOW() ELSE approved_at END,
				updated_at = NOW()
			WHERE organization_id = $1 AND id = $2 AND status = 'sent' AND deleted_at IS NULL
			RETURNING id::text`, scope.OrganizationID(), p.QuotationID, string(newStatus)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrQuotationNotRespondable
		}
		if err != nil {
			return err
		}

		response, err = scanQuotationResponse(tx.QueryRow(ctx, `
			INSERT INTO crm_quotation_responses (
				organization_id, quotation_id, link_id, action, revision_categories, note, responder_name, ip, user_agent
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9)
			RETURNING `+quotationResponseColumns,
			scope.OrganizationID(), p.QuotationID, nullableString(p.LinkID), string(p.Action),
			nullableStrings(categories), nullableString(p.Note), p.ResponderName,
			nullableString(p.IP), nullableString(p.UserAgent)))
		if err != nil {
			return err
		}

		quotation, err = scanQuotation(tx.QueryRow(ctx,
			`SELECT `+quotationColumns+` FROM crm_quotations WHERE organization_id = $1 AND id = $2`,
			scope.OrganizationID(), p.QuotationID))
		if err != nil {
			return err
		}
		quotation.Items, err = loadQuotationItems(ctx, tx, scope.OrganizationID(), quotation.ID)
		return err
	})
	if err != nil {
		return domain.Quotation{}, domain.QuotationResponse{}, err
	}
	return quotation, response, nil
}

func (r *quotationResponseRepository) ListByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) ([]domain.QuotationResponse, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	out := []domain.QuotationResponse{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+quotationResponseColumns+` FROM crm_quotation_responses
			WHERE organization_id = $1 AND quotation_id = $2 ORDER BY created_at DESC, id DESC`,
			scope.OrganizationID(), quotationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			resp, err := scanQuotationResponse(rows)
			if err != nil {
				return err
			}
			out = append(out, resp)
		}
		return rows.Err()
	})
	return out, err
}

// nullableStrings: slice kosong disimpan sebagai NULL.
func nullableStrings(v []string) interface{} {
	if len(v) == 0 {
		return nil
	}
	return v
}
