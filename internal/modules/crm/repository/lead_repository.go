package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type leadRepository struct {
	db *database.Pool
}

func NewLeadRepository(db *database.Pool) LeadRepository {
	return &leadRepository{db: db}
}

func (r *leadRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

// insertLeadEvent writes a crm_lead_events row inside the caller's
// transaction, so the history can never disagree with crm_leads.
func insertLeadEvent(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, leadID string, eventType domain.LeadEventType, from, to, actorUserID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO crm_lead_events (organization_id, lead_id, event_type, from_value, to_value, actor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		scope.OrganizationID(), leadID, string(eventType),
		nullableString(from), nullableString(to), nullableString(actorUserID),
	)
	return err
}

// lockLeadState reads the fields whose changes are recorded as events and
// locks the row for the rest of the transaction.
func lockLeadState(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, id string) (status string, ownerUserID string, err error) {
	var owner *string
	err = tx.QueryRow(ctx, `
		SELECT status, owner_user_id FROM crm_leads
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		FOR UPDATE`, id, scope.OrganizationID()).Scan(&status, &owner)
	if owner != nil {
		ownerUserID = *owner
	}
	return status, ownerUserID, err
}

// owner_name dibatasi ke user yang (pernah) menjadi anggota organization lead
// ini, supaya owner_user_id asing tidak bisa dipakai untuk membaca nama user
// tenant lain. Dipakai juga di klausa RETURNING, jadi referensi tabel luar
// ditulis eksplisit sebagai crm_leads.
const leadColumns = `
	id, organization_id, contact_name, company_name, email, phone, source, status, score,
	owner_user_id, notes, converted_contact_id, converted_company_id, converted_deal_id, converted_at,
	created_by, updated_by, created_at, updated_at, deleted_at,
	job_title, annual_revenue::text, address,
	requirement_summary, budget_estimate::text, target_date, decision_maker, disqualify_reason, disqualify_note,
	(
		SELECT u.name FROM users u
		JOIN organization_memberships m
			ON m.user_id = u.id AND m.organization_id = crm_leads.organization_id
		WHERE u.id = crm_leads.owner_user_id
	) AS owner_name
`

func scanLead(row pgx.Row) (domain.Lead, error) {
	var l domain.Lead
	var companyName, email, phone, source, notes *string
	var ownerUserID, createdBy, updatedBy *string
	var jobTitle, ownerName *string
	var requirementSummary, decisionMaker, disqualifyReason, disqualifyNote *string
	var status string

	err := row.Scan(
		&l.ID, &l.OrganizationID, &l.ContactName, &companyName, &email, &phone, &source, &status, &l.Score,
		&ownerUserID, &notes, &l.ConvertedContactID, &l.ConvertedCompanyID, &l.ConvertedDealID, &l.ConvertedAt,
		&createdBy, &updatedBy, &l.CreatedAt, &l.UpdatedAt, &l.DeletedAt,
		&jobTitle, &l.AnnualRevenue, &l.Address,
		&requirementSummary, &l.BudgetEstimate, &l.TargetDate, &decisionMaker, &disqualifyReason, &disqualifyNote,
		&ownerName,
	)
	if err != nil {
		return domain.Lead{}, err
	}

	l.Status = domain.LeadStatus(status)
	if companyName != nil {
		l.CompanyName = *companyName
	}
	if email != nil {
		l.Email = *email
	}
	if phone != nil {
		l.Phone = *phone
	}
	if source != nil {
		l.Source = *source
	}
	if notes != nil {
		l.Notes = *notes
	}
	if ownerUserID != nil {
		l.OwnerUserID = *ownerUserID
	}
	if ownerName != nil {
		l.OwnerName = *ownerName
	}
	if jobTitle != nil {
		l.JobTitle = *jobTitle
	}
	if requirementSummary != nil {
		l.RequirementSummary = *requirementSummary
	}
	if decisionMaker != nil {
		l.DecisionMaker = *decisionMaker
	}
	if disqualifyReason != nil {
		l.DisqualifyReason = *disqualifyReason
	}
	if disqualifyNote != nil {
		l.DisqualifyNote = *disqualifyNote
	}
	if l.Address == nil {
		l.Address = map[string]any{}
	}
	if createdBy != nil {
		l.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		l.UpdatedBy = *updatedBy
	}

	return l, nil
}

func (r *leadRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateLeadParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	address := params.Address
	if address == nil {
		address = map[string]any{}
	}

	query := `
		INSERT INTO crm_leads (
			organization_id, contact_name, company_name, email, phone, source, status, score,
			owner_user_id, notes, created_by, job_title, annual_revenue, address
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'new', $7, $8, $9, $10, $11, $12, $13
		) RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.ContactName,
			nullableString(params.CompanyName),
			nullableString(params.Email),
			nullableString(params.Phone),
			nullableString(params.Source),
			params.Score,
			nullableString(params.OwnerUserID),
			nullableString(params.Notes),
			nullableString(params.CreatedBy),
			nullableString(params.JobTitle),
			nullableString(params.AnnualRevenue),
			address,
		))
		if scanErr != nil {
			return scanErr
		}
		if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventCreated, "", string(lead.Status), params.CreatedBy); err != nil {
			return err
		}
		if params.SkipPlaybook {
			return nil
		}
		_, err := startLeadRunTx(ctx, tx, scope, lead, params.CreatedBy, time.Now())
		return err
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + leadColumns + ` FROM crm_leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) FindActiveByPhone(ctx context.Context, scope coretenant.Scope, phoneNormalized string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + leadColumns + ` FROM crm_leads
		WHERE organization_id = $1 AND phone_normalized = $2 AND deleted_at IS NULL AND status <> 'converted'
		ORDER BY updated_at DESC
		LIMIT 1`

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, scope.OrganizationID(), phoneNormalized))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) FindOpenByEmail(ctx context.Context, scope coretenant.Scope, email string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + leadColumns + ` FROM crm_leads
		WHERE organization_id = $1 AND lower(email) = lower($2)
		AND status NOT IN ('converted', 'unqualified') AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, scope.OrganizationID(), strings.TrimSpace(email)))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) List(ctx context.Context, scope coretenant.Scope, filter LeadListFilter) ([]domain.Lead, int64, error) {
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
		idx := len(args)
		whereClauses = append(whereClauses, fmt.Sprintf("(contact_name ILIKE $%d OR company_name ILIKE $%d OR email ILIKE $%d)", idx, idx, idx))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.OwnerUserID != "" {
		args = append(args, filter.OwnerUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("owner_user_id = $%d", len(args)))
	}
	if filter.ConvertedContactID != "" {
		args = append(args, filter.ConvertedContactID)
		whereClauses = append(whereClauses, fmt.Sprintf("converted_contact_id = $%d", len(args)))
	}
	if filter.Source != "" {
		args = append(args, strings.ToLower(strings.TrimSpace(filter.Source)))
		whereClauses = append(whereClauses, fmt.Sprintf("lower(btrim(source)) = $%d", len(args)))
	}
	if !filter.CreatedFrom.IsZero() {
		args = append(args, filter.CreatedFrom)
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if !filter.CreatedToExclusive.IsZero() {
		args = append(args, filter.CreatedToExclusive)
		whereClauses = append(whereClauses, fmt.Sprintf("created_at < $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_leads WHERE " + where

	var leads []domain.Lead
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			leads = []domain.Lead{}
			return nil
		}

		query := "SELECT " + leadColumns + " FROM crm_leads WHERE " + where + " ORDER BY " + leadOrderBy(filter.Sort)
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
			lead, scanErr := scanLead(rows)
			if scanErr != nil {
				return scanErr
			}
			leads = append(leads, lead)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return leads, total, nil
}

func (r *leadRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateLeadParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}

	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.ContactName != nil {
		addSet("contact_name", *params.ContactName)
	}
	if params.CompanyName != nil {
		addSet("company_name", *params.CompanyName)
	}
	if params.Email != nil {
		addSet("email", *params.Email)
	}
	if params.Phone != nil {
		addSet("phone", *params.Phone)
	}
	if params.Source != nil {
		addSet("source", *params.Source)
	}
	if params.Status != nil {
		addSet("status", string(*params.Status))
	}
	if params.Score != nil {
		addSet("score", *params.Score)
	}
	if params.OwnerUserID != nil {
		addSet("owner_user_id", nullableString(*params.OwnerUserID))
	}
	if params.Notes != nil {
		addSet("notes", *params.Notes)
	}
	if params.JobTitle != nil {
		addSet("job_title", nullableString(*params.JobTitle))
	}
	if params.AnnualRevenue != nil {
		// String kosong = hapus nilai (NULL), bukan 0.
		addSet("annual_revenue", nullableString(*params.AnnualRevenue))
	}
	if params.Address != nil {
		addSet("address", params.Address)
	}
	if params.RequirementSummary != nil {
		addSet("requirement_summary", nullableString(strings.TrimSpace(*params.RequirementSummary)))
	}
	if params.BudgetEstimate != nil {
		addSet("budget_estimate", nullableString(*params.BudgetEstimate))
	}
	if params.TargetDate != nil {
		addSet("target_date", nullableString(*params.TargetDate))
	}
	if params.DecisionMaker != nil {
		addSet("decision_maker", nullableString(strings.TrimSpace(*params.DecisionMaker)))
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_leads SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		oldStatus, oldOwner, err := lockLeadState(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		lead, err = scanLead(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return err
		}
		if string(lead.Status) != oldStatus {
			if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventStatusChanged, oldStatus, string(lead.Status), params.UpdatedBy); err != nil {
				return err
			}
			if err := endRunIfStatusLeavesStep(ctx, tx, scope, lead, params.UpdatedBy); err != nil {
				return err
			}
		}
		if lead.OwnerUserID != oldOwner {
			if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventAssigned, oldOwner, lead.OwnerUserID, params.UpdatedBy); err != nil {
				return err
			}
			return reassignLeadRunTx(ctx, tx, scope, lead.ID, leadPIC(lead), params.UpdatedBy)
		}
		return nil
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
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
		if err := insertLeadEvent(ctx, tx, scope, id, domain.LeadEventDeleted, "", "", deletedBy); err != nil {
			return err
		}
		return endLeadRunTx(ctx, tx, scope, id, domain.PlaybookResultCancelled, deletedBy)
	})
}

func (r *leadRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
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
		return insertLeadEvent(ctx, tx, scope, id, domain.LeadEventRestored, "", "", restoredBy)
	})
}

func (r *leadRepository) Assign(ctx context.Context, scope coretenant.Scope, id string, ownerUserID string, updatedBy string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
		SET owner_user_id = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		_, oldOwner, err := lockLeadState(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		lead, err = scanLead(tx.QueryRow(ctx, query, ownerUserID, nullableString(updatedBy), id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		if lead.OwnerUserID == oldOwner {
			return nil
		}
		if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventAssigned, oldOwner, lead.OwnerUserID, updatedBy); err != nil {
			return err
		}
		return reassignLeadRunTx(ctx, tx, scope, lead.ID, leadPIC(lead), updatedBy)
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) MarkConverted(ctx context.Context, scope coretenant.Scope, id string, params MarkConvertedParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
		SET status = 'converted',
			converted_contact_id = $1,
			converted_company_id = $2,
			converted_at = NOW(),
			updated_by = $3,
			updated_at = NOW()
		WHERE id = $4 AND organization_id = $5 AND deleted_at IS NULL AND status <> 'converted'
		RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		oldStatus, _, err := lockLeadState(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		lead, err = scanLead(tx.QueryRow(ctx, query,
			params.ConvertedContactID,
			nullableString(params.ConvertedCompanyID),
			nullableString(params.UpdatedBy),
			id,
			scope.OrganizationID(),
		))
		if err != nil {
			return err
		}
		if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventConverted, oldStatus, string(lead.Status), params.UpdatedBy); err != nil {
			return err
		}
		return endLeadRunTx(ctx, tx, scope, lead.ID, domain.PlaybookResultConverted, params.UpdatedBy)
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) Disqualify(ctx context.Context, scope coretenant.Scope, id string, params DisqualifyLeadParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}
	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		oldStatus, _, err := lockLeadState(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		lead, err = scanLead(tx.QueryRow(ctx, `
			UPDATE crm_leads SET status = 'unqualified', disqualify_reason = $1, disqualify_note = $2,
				updated_by = $3, updated_at = NOW()
			WHERE id = $4 AND organization_id = $5 AND deleted_at IS NULL AND status <> 'converted'
			RETURNING `+leadColumns,
			string(params.Reason), nullableString(strings.TrimSpace(params.Note)), nullableString(params.UpdatedBy), id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		if oldStatus != string(lead.Status) {
			if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventStatusChanged, oldStatus, string(lead.Status), params.UpdatedBy); err != nil {
				return err
			}
		}
		return endLeadRunTx(ctx, tx, scope, lead.ID, domain.PlaybookResultDisqualified, params.UpdatedBy)
	})
	return lead, err
}

func (r *leadRepository) StartPlaybook(ctx context.Context, scope coretenant.Scope, id string, startedBy string) (domain.PlaybookRun, error) {
	if !scope.IsValid() {
		return domain.PlaybookRun{}, coretenant.ErrInvalidScope
	}
	var run domain.PlaybookRun
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if _, _, err := lockLeadState(ctx, tx, scope, id); err != nil {
			return err
		}
		lead, err := scanLead(tx.QueryRow(ctx, `SELECT `+leadColumns+` FROM crm_leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`, id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		if _, active, err := findActiveRun(ctx, tx, scope, id); err != nil {
			return err
		} else if active {
			return ErrPlaybookAlreadyActive
		}
		enabled, err := leadPlaybookEnabled(ctx, tx, scope.OrganizationID())
		if err != nil {
			return err
		}
		if !enabled {
			return ErrPlaybookDisabled
		}
		started, err := startLeadRunTx(ctx, tx, scope, lead, startedBy, time.Now())
		if err != nil {
			return err
		}
		if started == nil {
			return ErrPlaybookNotApplicable
		}
		run = *started
		return nil
	})
	return run, err
}

func (r *leadRepository) FindPlaybookSummaries(ctx context.Context, scope coretenant.Scope, leadIDs []string) (map[string]domain.LeadPlaybookSummary, error) {
	out := map[string]domain.LeadPlaybookSummary{}
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	if len(leadIDs) == 0 {
		return out, nil
	}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ON (r.entity_id)
				r.entity_id, r.id, r.status, r.result, COALESCE(s.key, ''), COALESCE(s.name, ''),
				a.due_at, COALESCE(a.attempt_no, 0), COALESCE(a.final_review, false)
			FROM crm_playbook_runs r
			LEFT JOIN crm_playbook_steps s ON s.id = r.current_step_id
			LEFT JOIN crm_activities a ON a.organization_id = r.organization_id AND a.playbook_run_id = r.id
				AND a.status = 'pending' AND a.deleted_at IS NULL
			WHERE r.organization_id = $1 AND r.entity_type = 'lead' AND r.entity_id = ANY($2::uuid[])
			ORDER BY r.entity_id, (r.status = 'active') DESC, r.started_at DESC`,
			scope.OrganizationID(), leadIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var leadID, status string
			var result *string
			var s domain.LeadPlaybookSummary
			if err := rows.Scan(&leadID, &s.RunID, &status, &result, &s.StepKey, &s.StepName, &s.DueAt, &s.AttemptNo, &s.FinalReview); err != nil {
				return err
			}
			s.Status = domain.PlaybookRunStatus(status)
			if result != nil {
				rr := domain.PlaybookRunResult(*result)
				s.Result = &rr
			}
			out[leadID] = s
		}
		return rows.Err()
	})
	return out, err
}

func (r *leadRepository) ListEvents(ctx context.Context, scope coretenant.Scope, leadID string, limit, offset int) ([]domain.LeadEvent, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}
	events := []domain.LeadEvent{}
	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM crm_leads WHERE id = $1 AND organization_id = $2)`, leadID, scope.OrganizationID()).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM crm_lead_events WHERE organization_id = $1 AND lead_id = $2`, scope.OrganizationID(), leadID).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, e.lead_id, e.event_type, COALESCE(e.from_value, ''), COALESCE(e.to_value, ''),
				COALESCE(e.actor_user_id::text, ''), COALESCE(`+memberName("e.actor_user_id")+`, ''),
				CASE WHEN e.event_type = 'assigned' AND COALESCE(e.from_value, '') <> '' THEN COALESCE(`+memberName("e.from_value::uuid")+`, '') ELSE '' END,
				CASE WHEN e.event_type = 'assigned' AND COALESCE(e.to_value, '') <> '' THEN COALESCE(`+memberName("e.to_value::uuid")+`, '') ELSE '' END,
				e.created_at
			FROM crm_lead_events e
			WHERE e.organization_id = $1 AND e.lead_id = $2
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT $3 OFFSET $4`, scope.OrganizationID(), leadID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ev domain.LeadEvent
			var eventType string
			if err := rows.Scan(&ev.ID, &ev.LeadID, &eventType, &ev.FromValue, &ev.ToValue, &ev.ActorUserID, &ev.ActorName, &ev.FromName, &ev.ToName, &ev.CreatedAt); err != nil {
				return err
			}
			ev.EventType = domain.LeadEventType(eventType)
			events = append(events, ev)
		}
		return rows.Err()
	})
	return events, total, err
}
