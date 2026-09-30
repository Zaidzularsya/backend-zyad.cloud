package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/playbook"
)

const leadPlaybookTrigger = "lead_created"

func leadPIC(lead domain.Lead) string {
	if lead.OwnerUserID != "" {
		return lead.OwnerUserID
	}
	return lead.CreatedBy
}

func leadPlaybookEnabled(ctx context.Context, tx pgx.Tx, orgID string) (bool, error) {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT lead_playbook_enabled FROM crm_settings WHERE organization_id = $1), true)`, orgID).Scan(&enabled)
	return enabled, err
}

// loadActivePlaybook prefers an organization-specific playbook over the
// system default (organization_id NULL), highest active version first.
func loadActivePlaybook(ctx context.Context, tx pgx.Tx, orgID string) (domain.Playbook, bool, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM crm_playbooks
		WHERE entity_type = 'lead' AND trigger = $2 AND is_active
			AND (organization_id = $1 OR organization_id IS NULL)
		ORDER BY organization_id NULLS LAST, version DESC
		LIMIT 1`, orgID, leadPlaybookTrigger).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Playbook{}, false, nil
	}
	if err != nil {
		return domain.Playbook{}, false, err
	}
	pb, err := loadPlaybookByID(ctx, tx, id)
	return pb, err == nil, err
}

func loadPlaybookByID(ctx context.Context, tx pgx.Tx, id string) (domain.Playbook, error) {
	var pb domain.Playbook
	err := tx.QueryRow(ctx, `SELECT id, organization_id, entity_type, trigger, key, name, version FROM crm_playbooks WHERE id = $1`, id).
		Scan(&pb.ID, &pb.OrganizationID, &pb.EntityType, &pb.Trigger, &pb.Key, &pb.Name, &pb.Version)
	if err != nil {
		return domain.Playbook{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, key, name, activity_type, due_rule, channel_actions, entry_statuses, max_attempts, position
		FROM crm_playbook_steps WHERE playbook_id = $1 ORDER BY position`, id)
	if err != nil {
		return domain.Playbook{}, err
	}
	for rows.Next() {
		var s domain.PlaybookStep
		var activityType string
		var dueRule []byte
		var entry []string
		if err := rows.Scan(&s.ID, &s.Key, &s.Name, &activityType, &dueRule, &s.ChannelActions, &entry, &s.MaxAttempts, &s.Position); err != nil {
			rows.Close()
			return domain.Playbook{}, err
		}
		s.PlaybookID = id
		s.ActivityType = domain.ActivityType(activityType)
		if err := json.Unmarshal(dueRule, &s.DueRule); err != nil {
			rows.Close()
			return domain.Playbook{}, err
		}
		for _, st := range entry {
			s.EntryStatuses = append(s.EntryStatuses, domain.LeadStatus(st))
		}
		pb.Steps = append(pb.Steps, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return domain.Playbook{}, err
	}

	rows, err = tx.Query(ctx, `
		SELECT o.id, o.step_id, o.key, o.label, o.required_input, o.set_lead_status, o.disqualify_reason,
			o.next_step_key, o.next_due_rule, o.repeat_step, o.ends_run, o.run_result, o.visibility, o.position
		FROM crm_playbook_outcomes o JOIN crm_playbook_steps s ON s.id = o.step_id
		WHERE s.playbook_id = $1 ORDER BY s.position, o.position`, id)
	if err != nil {
		return domain.Playbook{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var o domain.PlaybookOutcome
		var requiredInput, visibility string
		var setStatus, reason, runResult *string
		var nextDue []byte
		if err := rows.Scan(&o.ID, &o.StepID, &o.Key, &o.Label, &requiredInput, &setStatus, &reason,
			&o.NextStepKey, &nextDue, &o.RepeatStep, &o.EndsRun, &runResult, &visibility, &o.Position); err != nil {
			return domain.Playbook{}, err
		}
		o.RequiredInput = domain.PlaybookInput(requiredInput)
		o.Visibility = domain.OutcomeVisibility(visibility)
		if setStatus != nil {
			st := domain.LeadStatus(*setStatus)
			o.SetLeadStatus = &st
		}
		if reason != nil {
			r := domain.DisqualifyReason(*reason)
			o.DisqualifyReason = &r
		}
		if runResult != nil {
			rr := domain.PlaybookRunResult(*runResult)
			o.RunResult = &rr
		}
		if nextDue != nil {
			var rule domain.DueRule
			if err := json.Unmarshal(nextDue, &rule); err != nil {
				return domain.Playbook{}, err
			}
			o.NextDueRule = &rule
		}
		for i := range pb.Steps {
			if pb.Steps[i].ID == o.StepID {
				pb.Steps[i].Outcomes = append(pb.Steps[i].Outcomes, o)
			}
		}
	}
	return pb, rows.Err()
}

const runColumns = `id, organization_id, playbook_id, playbook_version, entity_type, entity_id, current_step_id, status, result, started_by, started_at, ended_at`

func scanRun(row pgx.Row) (domain.PlaybookRun, error) {
	var r domain.PlaybookRun
	var status string
	var result, startedBy *string
	err := row.Scan(&r.ID, &r.OrganizationID, &r.PlaybookID, &r.PlaybookVersion, &r.EntityType, &r.EntityID,
		&r.CurrentStepID, &status, &result, &startedBy, &r.StartedAt, &r.EndedAt)
	if err != nil {
		return domain.PlaybookRun{}, err
	}
	r.Status = domain.PlaybookRunStatus(status)
	if result != nil {
		rr := domain.PlaybookRunResult(*result)
		r.Result = &rr
	}
	if startedBy != nil {
		r.StartedBy = *startedBy
	}
	return r, nil
}

func findActiveRun(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, leadID string) (domain.PlaybookRun, bool, error) {
	run, err := scanRun(tx.QueryRow(ctx, `SELECT `+runColumns+` FROM crm_playbook_runs
		WHERE organization_id = $1 AND entity_type = 'lead' AND entity_id = $2 AND status = 'active'
		FOR UPDATE`, scope.OrganizationID(), leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PlaybookRun{}, false, nil
	}
	return run, err == nil, err
}

// startLeadRunTx starts the default playbook for lead (R1/R8). Returns
// (nil, nil) when the organization disabled it or no playbook exists, so
// lead creation never fails because of the playbook. Manual start maps
// those cases to errors itself.
func startLeadRunTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, lead domain.Lead, startedBy string, now time.Time) (*domain.PlaybookRun, error) {
	enabled, err := leadPlaybookEnabled(ctx, tx, scope.OrganizationID())
	if err != nil || !enabled {
		return nil, err
	}
	pb, ok, err := loadActivePlaybook(ctx, tx, scope.OrganizationID())
	if err != nil || !ok {
		return nil, err
	}
	step, ok := playbook.StepForStatus(pb, lead.Status)
	if !ok {
		return nil, nil
	}
	run, err := scanRun(tx.QueryRow(ctx, `
		INSERT INTO crm_playbook_runs (organization_id, playbook_id, playbook_version, entity_type, entity_id, current_step_id, status, started_by)
		VALUES ($1, $2, $3, 'lead', $4, $5, 'active', $6)
		RETURNING `+runColumns,
		scope.OrganizationID(), pb.ID, pb.Version, lead.ID, step.ID, nullableString(startedBy)))
	if err != nil {
		return nil, err
	}
	next := playbook.NextStep{Step: step, AttemptNo: 1, DueAt: playbook.ComputeDue(step.DueRule, 1, now)}
	if _, err := insertPlaybookActivityTx(ctx, tx, scope, run, next, leadPIC(lead), startedBy); err != nil {
		return nil, err
	}
	if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventPlaybookStarted, "", step.Key, startedBy); err != nil {
		return nil, err
	}
	return &run, nil
}

func insertPlaybookActivityTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, run domain.PlaybookRun, next playbook.NextStep, assignee, createdBy string) (domain.Activity, error) {
	subject := next.Step.Name
	if next.FinalReview {
		subject = "Tinjau: lead tidak responsif"
	}
	a, err := scanActivity(tx.QueryRow(ctx, `
		INSERT INTO crm_activities (
			organization_id, related_entity_type, related_entity_id, type, subject, due_at,
			assignee_user_id, created_by, playbook_run_id, playbook_step_id, attempt_no, final_review
		) VALUES ($1, 'lead', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+activityColumns,
		scope.OrganizationID(), run.EntityID, string(next.Step.ActivityType), subject, next.DueAt,
		nullableString(assignee), nullableString(createdBy), run.ID, next.Step.ID, next.AttemptNo, next.FinalReview))
	if err != nil {
		return domain.Activity{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE crm_playbook_runs SET current_step_id = $1 WHERE id = $2 AND organization_id = $3`,
		next.Step.ID, run.ID, scope.OrganizationID())
	return a, err
}

// endLeadRunTx ends the lead's active run (no-op when none) and cancels its
// pending activities (R6).
func endLeadRunTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, leadID string, result domain.PlaybookRunResult, actor string) error {
	run, ok, err := findActiveRun(ctx, tx, scope, leadID)
	if err != nil || !ok {
		return err
	}
	status := domain.PlaybookRunCompleted
	if result == domain.PlaybookResultCancelled {
		status = domain.PlaybookRunCancelled
	}
	if _, err := tx.Exec(ctx, `UPDATE crm_playbook_runs SET status = $1, result = $2, ended_at = NOW()
		WHERE id = $3 AND organization_id = $4`, string(status), string(result), run.ID, scope.OrganizationID()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE crm_activities SET status = 'cancelled', updated_by = $1, updated_at = NOW()
		WHERE organization_id = $2 AND playbook_run_id = $3 AND status = 'pending' AND deleted_at IS NULL`,
		nullableString(actor), scope.OrganizationID(), run.ID); err != nil {
		return err
	}
	return insertLeadEvent(ctx, tx, scope, leadID, domain.LeadEventPlaybookEnded, "", string(result), actor)
}

// reassignLeadRunTx moves the pending playbook step to the lead's new PIC (R7).
func reassignLeadRunTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, leadID, assignee, actor string) error {
	_, err := tx.Exec(ctx, `
		UPDATE crm_activities a SET assignee_user_id = $1, updated_by = $2, updated_at = NOW()
		FROM crm_playbook_runs r
		WHERE r.organization_id = $3 AND r.entity_type = 'lead' AND r.entity_id = $4 AND r.status = 'active'
			AND a.organization_id = r.organization_id AND a.playbook_run_id = r.id
			AND a.status = 'pending' AND a.deleted_at IS NULL`,
		nullableString(assignee), nullableString(actor), scope.OrganizationID(), leadID)
	return err
}

// attachPlaybookInfo fills Activity.Playbook for playbook activities.
// Definitions are few and global, so they are cached per call.
func attachPlaybookInfo(ctx context.Context, tx pgx.Tx, activities []domain.Activity) error {
	cache := map[string]domain.Playbook{}
	for i := range activities {
		a := &activities[i]
		if a.PlaybookStepID == nil || a.PlaybookRunID == nil {
			continue
		}
		var playbookID string
		if err := tx.QueryRow(ctx, `SELECT playbook_id FROM crm_playbook_steps WHERE id = $1`, *a.PlaybookStepID).Scan(&playbookID); err != nil {
			return err
		}
		pb, ok := cache[playbookID]
		if !ok {
			loaded, err := loadPlaybookByID(ctx, tx, playbookID)
			if err != nil {
				return err
			}
			pb, cache[playbookID] = loaded, loaded
		}
		for _, s := range pb.Steps {
			if s.ID != *a.PlaybookStepID {
				continue
			}
			attempt := 1
			if a.AttemptNo != nil {
				attempt = *a.AttemptNo
			}
			a.Playbook = &domain.ActivityPlaybookInfo{
				RunID: *a.PlaybookRunID, StepKey: s.Key, StepName: s.Name, AttemptNo: attempt,
				MaxAttempts: s.MaxAttempts, FinalReview: a.FinalReview, ChannelActions: s.ChannelActions,
				Outcomes: playbook.AvailableOutcomes(s, a.FinalReview),
			}
		}
	}
	return nil
}
func endRunIfStatusLeavesStep(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, lead domain.Lead, actor string) error {
	run, ok, err := findActiveRun(ctx, tx, scope, lead.ID)
	if err != nil || !ok || run.CurrentStepID == nil {
		return err
	}
	var entry []string
	if err := tx.QueryRow(ctx, `SELECT entry_statuses FROM crm_playbook_steps WHERE id = $1`, *run.CurrentStepID).Scan(&entry); err != nil {
		return err
	}
	for _, s := range entry {
		if s == string(lead.Status) {
			return nil
		}
	}
	return endLeadRunTx(ctx, tx, scope, lead.ID, domain.PlaybookResultCancelled, actor)
}

// applyTransitionToLead writes status / requirements / disqualify fields of
// a transition and records the status_changed event.
func applyTransitionToLead(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, leadID string, tr playbook.Transition, actor string) (domain.Lead, error) {
	oldStatus, _, err := lockLeadState(ctx, tx, scope, leadID)
	if err != nil {
		return domain.Lead{}, err
	}
	sets := []string{"updated_at = NOW()"}
	args := []any{}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if tr.SetLeadStatus != nil {
		add("status", string(*tr.SetLeadStatus))
	}
	if tr.DisqualifyReason != nil {
		add("disqualify_reason", string(*tr.DisqualifyReason))
		add("disqualify_note", nullableString(tr.DisqualifyNote))
	}
	if r := tr.Requirements; r != nil {
		add("requirement_summary", r.Summary)
		if r.BudgetEstimate != nil {
			add("budget_estimate", nullableString(*r.BudgetEstimate))
		}
		if r.TargetDate != nil {
			add("target_date", r.TargetDate.Format("2006-01-02"))
		}
		add("decision_maker", nullableString(r.DecisionMaker))
	}
	if actor != "" {
		add("updated_by", actor)
	}
	args = append(args, leadID, scope.OrganizationID())
	lead, err := scanLead(tx.QueryRow(ctx, "UPDATE crm_leads SET "+strings.Join(sets, ", ")+
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args))+leadColumns, args...))
	if err != nil {
		return domain.Lead{}, err
	}
	if string(lead.Status) != oldStatus {
		if err := insertLeadEvent(ctx, tx, scope, lead.ID, domain.LeadEventStatusChanged, oldStatus, string(lead.Status), actor); err != nil {
			return domain.Lead{}, err
		}
	}
	return lead, nil
}
