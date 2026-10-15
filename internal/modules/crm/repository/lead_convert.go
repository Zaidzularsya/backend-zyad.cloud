package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

// ConvertLead membuat company (opsional), contact, deal (opsional) dan
// menandai lead converted dalam SATU transaksi; gagal di langkah mana pun
// membatalkan semuanya. Baris lead dikunci lebih dulu sehingga convert
// paralel kedua gagal dengan ErrLeadAlreadyConverted tanpa insert apa pun.
func (r *leadRepository) ConvertLead(ctx context.Context, scope coretenant.Scope, id string, params ConvertLeadTxParams) (domain.LeadConversionResult, error) {
	if !scope.IsValid() {
		return domain.LeadConversionResult{}, coretenant.ErrInvalidScope
	}
	var result domain.LeadConversionResult
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		oldStatus, _, err := lockLeadState(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if oldStatus == string(domain.LeadStatusConverted) {
			return ErrLeadAlreadyConverted
		}

		companyID := params.ExistingCompanyID
		switch {
		case params.NewCompany != nil:
			company, err := insertCompanyTx(ctx, tx, scope, *params.NewCompany)
			if err != nil {
				return err
			}
			result.Company = &company
			companyID = company.ID
		case companyID != "":
			company, err := scanCompany(tx.QueryRow(ctx,
				"SELECT "+companyColumns+" FROM crm_companies WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL",
				companyID, scope.OrganizationID()))
			if err != nil {
				return err
			}
			result.Company = &company
		}

		var contact domain.Contact
		if params.ExistingContactID != "" {
			contact, err = scanContact(tx.QueryRow(ctx,
				"SELECT "+contactColumns+" FROM crm_contacts WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL",
				params.ExistingContactID, scope.OrganizationID()))
		} else {
			contactParams := params.Contact
			contactParams.CompanyID = companyID
			contact, err = insertContactTx(ctx, tx, scope, contactParams)
		}
		if err != nil {
			return err
		}
		result.Contact = contact

		dealID := ""
		if params.Deal != nil {
			dealParams := *params.Deal
			dealParams.ContactID = contact.ID
			dealParams.CompanyID = companyID
			deal, err := insertDealTx(ctx, tx, scope, dealParams)
			if err != nil {
				return err
			}
			result.Deal = &deal
			dealID = deal.ID
		}

		result.Lead, err = scanLead(tx.QueryRow(ctx, `
			UPDATE crm_leads
			SET status = 'converted', converted_contact_id = $1, converted_company_id = $2,
				converted_deal_id = $3, converted_at = NOW(), updated_by = $4, updated_at = NOW()
			WHERE id = $5 AND organization_id = $6 AND deleted_at IS NULL
			RETURNING `+leadColumns,
			contact.ID, nullableString(companyID), nullableString(dealID),
			nullableString(params.UpdatedBy), id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		if err := insertLeadEvent(ctx, tx, scope, id, domain.LeadEventConverted, oldStatus, string(domain.LeadStatusConverted), params.UpdatedBy); err != nil {
			return err
		}
		return endLeadRunTx(ctx, tx, scope, id, domain.PlaybookResultConverted, params.UpdatedBy)
	})
	if err != nil {
		return domain.LeadConversionResult{}, err
	}
	return result, nil
}

// AttachDeal membuat deal untuk lead yang sudah converted tanpa deal
// (lead lama atau yang dulu tidak mencentang "Buat deal").
func (r *leadRepository) AttachDeal(ctx context.Context, scope coretenant.Scope, id string, params CreateDealParams) (domain.Lead, domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Lead{}, domain.Deal{}, coretenant.ErrInvalidScope
	}
	var lead domain.Lead
	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var status string
		var contactID, companyID, dealID *string
		err := tx.QueryRow(ctx, `
			SELECT status, converted_contact_id, converted_company_id, converted_deal_id
			FROM crm_leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
			FOR UPDATE`, id, scope.OrganizationID()).Scan(&status, &contactID, &companyID, &dealID)
		if err != nil {
			return err
		}
		if status != string(domain.LeadStatusConverted) {
			return ErrLeadNotConverted
		}
		if dealID != nil {
			return ErrLeadDealExists
		}
		if contactID != nil {
			params.ContactID = *contactID
		}
		if companyID != nil {
			params.CompanyID = *companyID
		}
		deal, err = insertDealTx(ctx, tx, scope, params)
		if err != nil {
			return err
		}
		lead, err = scanLead(tx.QueryRow(ctx, `
			UPDATE crm_leads SET converted_deal_id = $1, updated_by = $2, updated_at = NOW()
			WHERE id = $3 AND organization_id = $4
			RETURNING `+leadColumns,
			deal.ID, nullableString(params.CreatedBy), id, scope.OrganizationID()))
		return err
	})
	if err != nil {
		return domain.Lead{}, domain.Deal{}, err
	}
	return lead, deal, nil
}

// FindSourceLeadByDealID mengembalikan nil (tanpa error) bila deal tidak
// berasal dari lead.
func (r *leadRepository) FindSourceLeadByDealID(ctx context.Context, scope coretenant.Scope, dealID string) (*domain.DealSourceLead, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	var src domain.DealSourceLead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, contact_name FROM crm_leads
			WHERE organization_id = $1 AND converted_deal_id = $2 AND deleted_at IS NULL
			ORDER BY converted_at DESC LIMIT 1`, scope.OrganizationID(), dealID).Scan(&src.ID, &src.ContactName)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &src, nil
}
