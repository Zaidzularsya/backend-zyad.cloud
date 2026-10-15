package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type dealRepository struct {
	db *database.Pool
}

func NewDealRepository(db *database.Pool) DealRepository {
	return &dealRepository{db: db}
}

func (r *dealRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const dealColumns = `
	id, organization_id, pipeline_id, stage_id, company_id, contact_id, title,
	value::text, currency, expected_close_date, status, lost_reason, owner_user_id,
	discount_percent::text, discount_approved_by, discount_approved_at,
	created_by, updated_by, created_at, updated_at, deleted_at,
	description, decision_maker
`

func scanDeal(row pgx.Row) (domain.Deal, error) {
	var d domain.Deal
	var companyID, contactID *string
	var lostReason *string
	var ownerUserID, discountApprovedBy, createdBy, updatedBy *string
	var discountPercent *string
	var status string
	var description, decisionMaker *string

	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.PipelineID, &d.StageID, &companyID, &contactID, &d.Title,
		&d.Value, &d.Currency, &d.ExpectedCloseDate, &status, &lostReason, &ownerUserID,
		&discountPercent, &discountApprovedBy, &d.DiscountApprovedAt,
		&createdBy, &updatedBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&description, &decisionMaker,
	)
	if err != nil {
		return domain.Deal{}, err
	}

	d.Status = domain.DealStatus(status)
	d.CompanyID = companyID
	d.ContactID = contactID
	d.DiscountPercent = discountPercent
	if lostReason != nil {
		d.LostReason = *lostReason
	}
	if ownerUserID != nil {
		d.OwnerUserID = *ownerUserID
	}
	if discountApprovedBy != nil {
		d.DiscountApprovedBy = *discountApprovedBy
	}
	if createdBy != nil {
		d.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		d.UpdatedBy = *updatedBy
	}
	if description != nil {
		d.Description = *description
	}
	if decisionMaker != nil {
		d.DecisionMaker = *decisionMaker
	}

	return d, nil
}

func (r *dealRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateDealParams) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}
	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		deal, err = insertDealTx(ctx, tx, scope, params)
		return err
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func insertDealTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, params CreateDealParams) (domain.Deal, error) {
	currency := params.Currency
	if currency == "" {
		currency = "IDR"
	}
	value := params.Value
	if value == "" {
		value = "0"
	}
	return scanDeal(tx.QueryRow(ctx, `
		INSERT INTO crm_deals (
			organization_id, pipeline_id, stage_id, company_id, contact_id, title,
			value, currency, expected_close_date, owner_user_id, created_by,
			description, decision_maker
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING `+dealColumns,
		scope.OrganizationID(),
		params.PipelineID,
		params.StageID,
		nullableString(params.CompanyID),
		nullableString(params.ContactID),
		params.Title,
		value,
		currency,
		params.ExpectedCloseDate,
		nullableString(params.OwnerUserID),
		nullableString(params.CreatedBy),
		nullableString(strings.TrimSpace(params.Description)),
		nullableString(strings.TrimSpace(params.DecisionMaker)),
	))
}

func (r *dealRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + dealColumns + ` FROM crm_deals WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) List(ctx context.Context, scope coretenant.Scope, filter DealListFilter) ([]domain.Deal, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		whereClauses = append(whereClauses, fmt.Sprintf("title ILIKE $%d", len(args)))
	}
	if filter.PipelineID != "" {
		args = append(args, filter.PipelineID)
		whereClauses = append(whereClauses, fmt.Sprintf("pipeline_id = $%d", len(args)))
	}
	if filter.StageID != "" {
		args = append(args, filter.StageID)
		whereClauses = append(whereClauses, fmt.Sprintf("stage_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.OwnerUserID != "" {
		args = append(args, filter.OwnerUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("owner_user_id = $%d", len(args)))
	}
	if filter.ContactID != "" {
		args = append(args, filter.ContactID)
		whereClauses = append(whereClauses, fmt.Sprintf("contact_id = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_deals WHERE " + where

	var deals []domain.Deal
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			deals = []domain.Deal{}
			return nil
		}

		query := "SELECT " + dealColumns + " FROM crm_deals WHERE " + where + " ORDER BY created_at DESC"
		queryArgs := append([]interface{}{}, args...)
		if filter.Limit > 0 {
			queryArgs = append(queryArgs, filter.Limit)
			query += fmt.Sprintf(" LIMIT $%d", len(queryArgs))
		}
		if filter.Offset > 0 {
			queryArgs = append(queryArgs, filter.Offset)
			query += fmt.Sprintf(" OFFSET $%d", len(queryArgs))
		}

		rows, err := tx.Query(ctx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			deal, scanErr := scanDeal(rows)
			if scanErr != nil {
				return scanErr
			}
			deals = append(deals, deal)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return deals, total, nil
}

func (r *dealRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateDealParams) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}
	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.StageID != nil {
		addSet("stage_id", *params.StageID)
	}
	if params.CompanyID != nil {
		addSet("company_id", nullableString(*params.CompanyID))
	}
	if params.ContactID != nil {
		addSet("contact_id", nullableString(*params.ContactID))
	}
	if params.Title != nil {
		addSet("title", *params.Title)
	}
	if params.Value != nil {
		addSet("value", *params.Value)
	}
	if params.Currency != nil {
		addSet("currency", *params.Currency)
	}
	if params.ExpectedCloseDate != nil {
		addSet("expected_close_date", *params.ExpectedCloseDate)
	}
	if params.OwnerUserID != nil {
		addSet("owner_user_id", nullableString(*params.OwnerUserID))
	}
	if params.Description != nil {
		addSet("description", nullableString(strings.TrimSpace(*params.Description)))
	}
	if params.DecisionMaker != nil {
		addSet("decision_maker", nullableString(strings.TrimSpace(*params.DecisionMaker)))
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_deals SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		dealColumns

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *dealRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET deleted_at = NULL, updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NOT NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(restoredBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *dealRepository) MoveStage(ctx context.Context, scope coretenant.Scope, id string, stageID string, updatedBy string) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET stage_id = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL AND status = 'open'
		RETURNING ` + dealColumns

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, stageID, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) CloseWon(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET status = 'won', updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status = 'open'
		RETURNING ` + dealColumns

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) CloseLost(ctx context.Context, scope coretenant.Scope, id string, lostReason string, updatedBy string) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET status = 'lost', lost_reason = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL AND status = 'open'
		RETURNING ` + dealColumns

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, nullableString(lostReason), nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) ApproveDiscount(ctx context.Context, scope coretenant.Scope, id string, discountPercent string, approvedBy string) (domain.Deal, error) {
	if !scope.IsValid() {
		return domain.Deal{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_deals
		SET discount_percent = $1, discount_approved_by = $2, discount_approved_at = NOW(),
			updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		RETURNING ` + dealColumns

	var deal domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		deal, scanErr = scanDeal(tx.QueryRow(ctx, query, discountPercent, nullableString(approvedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Deal{}, err
	}
	return deal, nil
}

func (r *dealRepository) ListOpenByCompanyAndPipeline(ctx context.Context, scope coretenant.Scope, companyID, pipelineID string) ([]domain.Deal, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	var deals []domain.Deal
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+dealColumns+` FROM crm_deals
			WHERE organization_id = $1 AND company_id = $2 AND pipeline_id = $3
				AND status = 'open' AND deleted_at IS NULL
			ORDER BY created_at, id`, scope.OrganizationID(), companyID, pipelineID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			deal, err := scanDeal(rows)
			if err != nil {
				return err
			}
			deals = append(deals, deal)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return deals, nil
}
