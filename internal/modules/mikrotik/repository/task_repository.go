package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/platform/database"
)

// TaskFilter narrows List. Empty fields mean "any".
type TaskFilter struct {
	Status   string
	RouterID string
}

// TaskRepository is the outbox of isolate/restore tasks. All timestamps are
// passed in by the caller (UTC) so the worker controls the clock.
//
// Lease ownership is NOT checked by Release/Complete/RetryLater/Fail: the
// worker serialises work per router with an advisory lock, so only one worker
// handles a claimed task at a time. Each of these four returns
// mikrotik.ErrTaskNotFound when the task is no longer active (done, failed or
// cancelled), e.g. it was cancelled while running.
type TaskRepository interface {
	// EnqueueIsolate queues an isolate for an eligible secret (paired, present,
	// original profile known, normal state, router isolir_ready and not deleted)
	// and cancels a pending restore for it. enqueued=false (no error) means the
	// secret is not eligible OR an active task (e.g. a running opposite action)
	// already exists; callers must re-evaluate after the active task finishes.
	EnqueueIsolate(ctx context.Context, scope coretenant.Scope, secretID, event string) (bool, error)
	// EnqueueRestore queues a restore for an isolated secret and cancels a
	// pending isolate. It does not require the router to be isolir_ready.
	// enqueued=false has the same ambiguity as EnqueueIsolate: not eligible or an
	// active task (e.g. a running isolate, which is left untouched) exists.
	EnqueueRestore(ctx context.Context, scope coretenant.Scope, secretID, event string) (bool, error)
	// ClaimBatch leases up to limit due tasks (pending and due, or running with
	// an expired lease), restores first. Safe under concurrent callers. Reclaiming
	// an expired lease overwrites started_at and does not increase attempts.
	ClaimBatch(ctx context.Context, scope coretenant.Scope, limit int, now time.Time, lease time.Duration) ([]domain.IsolationTask, error)
	// Release returns a task to pending at next without counting an attempt.
	Release(ctx context.Context, scope coretenant.Scope, id string, next time.Time) error
	Complete(ctx context.Context, scope coretenant.Scope, id string) error
	// RetryLater returns a task to pending at next and counts an attempt.
	RetryLater(ctx context.Context, scope coretenant.Scope, id string, next time.Time, code, msg string) error
	Fail(ctx context.Context, scope coretenant.Scope, id, code, msg string) error
	// Cancel ends an active task; an already finished task is left untouched.
	Cancel(ctx context.Context, scope coretenant.Scope, id, reason string) error
	MarkTenantNotified(ctx context.Context, scope coretenant.Scope, id string) error
	MarkCustomerNotified(ctx context.Context, scope coretenant.Scope, id string) error
	// Retry moves a failed task back to pending with attempts reset;
	// ErrTaskNotRetryable for any other status, ErrTaskActiveExists when the
	// secret already has another pending/running task (the task stays failed).
	Retry(ctx context.Context, scope coretenant.Scope, id string) (domain.IsolationTask, error)
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.IsolationTask, error)
	List(ctx context.Context, scope coretenant.Scope, f TaskFilter, limit, offset int) ([]domain.TaskView, int64, error)
	CountByStatus(ctx context.Context, scope coretenant.Scope) (map[string]int, error)
}

type taskRepository struct {
	db *database.Pool
}

func NewTaskRepository(db *database.Pool) TaskRepository {
	return &taskRepository{db: db}
}

const taskColumns = `
	id, organization_id, secret_id, action, priority, status, attempts, next_attempt_at, locked_until,
	last_error, last_error_code, tenant_notified_at, customer_notified_at, created_by_event,
	created_at, started_at, finished_at
`

func scanTask(row pgx.Row) (domain.IsolationTask, error) {
	var t domain.IsolationTask
	err := row.Scan(&t.ID, &t.OrganizationID, &t.SecretID, &t.Action, &t.Priority, &t.Status, &t.Attempts,
		&t.NextAttemptAt, &t.LockedUntil, &t.LastError, &t.LastErrorCode, &t.TenantNotifiedAt, &t.CustomerNotifiedAt,
		&t.CreatedByEvent, &t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	return t, err
}

// Eligibility is repeated in the cancel and the insert statements so neither
// acts on a secret the other would refuse.
const (
	isolateEligible = `s.organization_id = $1 AND s.id = $2
		AND s.account_id IS NOT NULL AND s.present AND NOT s.needs_original_profile AND s.isolation_state = 'normal'
		AND EXISTS (SELECT 1 FROM mikrotik_routers r
			WHERE r.id = s.router_id AND r.organization_id = s.organization_id AND r.isolir_ready AND r.deleted_at IS NULL)`
	restoreEligible = `s.organization_id = $1 AND s.id = $2 AND s.isolation_state = 'isolated'`
)

func (r *taskRepository) enqueue(ctx context.Context, scope coretenant.Scope, secretID, event, action string, priority int, eligible, cancelAction string) (bool, error) {
	var enqueued bool
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE mikrotik_isolation_tasks t
			SET status = 'cancelled', finished_at = now(), locked_until = NULL, last_error = 'superseded'
			WHERE t.organization_id = $1 AND t.secret_id = $2 AND t.action = $3 AND t.status = 'pending'
			  AND EXISTS (SELECT 1 FROM mikrotik_ppp_secrets s WHERE `+eligible+`)`,
			scope.OrganizationID(), secretID, cancelAction); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO mikrotik_isolation_tasks (organization_id, secret_id, action, priority, created_by_event)
			SELECT s.organization_id, s.id, $3, $4, $5
			FROM mikrotik_ppp_secrets s
			WHERE `+eligible+`
			FOR UPDATE OF s
			ON CONFLICT DO NOTHING`,
			scope.OrganizationID(), secretID, action, priority, event)
		if err != nil {
			return err
		}
		enqueued = tag.RowsAffected() > 0
		return nil
	})
	return enqueued, err
}

func (r *taskRepository) EnqueueIsolate(ctx context.Context, scope coretenant.Scope, secretID, event string) (bool, error) {
	return r.enqueue(ctx, scope, secretID, event, domain.ActionIsolate, domain.PriorityIsolate, isolateEligible, domain.ActionRestore)
}

func (r *taskRepository) EnqueueRestore(ctx context.Context, scope coretenant.Scope, secretID, event string) (bool, error) {
	return r.enqueue(ctx, scope, secretID, event, domain.ActionRestore, domain.PriorityRestore, restoreEligible, domain.ActionIsolate)
}

func (r *taskRepository) ClaimBatch(ctx context.Context, scope coretenant.Scope, limit int, now time.Time, lease time.Duration) ([]domain.IsolationTask, error) {
	if limit <= 0 {
		return nil, nil
	}
	now = now.UTC()
	out := []domain.IsolationTask{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH picked AS (
				SELECT id FROM mikrotik_isolation_tasks
				WHERE organization_id = $1
				  AND ((status = 'pending' AND next_attempt_at <= $2)
				    OR (status = 'running' AND locked_until < $2))
				ORDER BY priority DESC, next_attempt_at, id
				LIMIT $3
				FOR UPDATE SKIP LOCKED
			)
			UPDATE mikrotik_isolation_tasks t
			SET status = 'running', locked_until = $4, started_at = $2
			FROM picked
			WHERE t.id = picked.id
			RETURNING `+prefixed("t", taskColumns),
			scope.OrganizationID(), now, limit, now.Add(lease))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, scanErr := scanTask(rows)
			if scanErr != nil {
				return scanErr
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	// UPDATE ... RETURNING does not preserve the picked order.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].NextAttemptAt.Before(out[j].NextAttemptAt)
	})
	return out, nil
}

// prefixed qualifies a comma-separated column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(strings.Join(strings.Fields(cols), " "), ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// updateActive applies sql to one still-active (pending/running) task of the
// organization; $1 = organization, $2 = task id, extra args follow.
func (r *taskRepository) updateActive(ctx context.Context, scope coretenant.Scope, id, setSQL string, args ...interface{}) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		all := append([]interface{}{scope.OrganizationID(), id}, args...)
		tag, err := tx.Exec(ctx, `UPDATE mikrotik_isolation_tasks SET `+setSQL+`
			WHERE organization_id = $1 AND id = $2 AND status IN ('pending', 'running')`, all...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return mikrotik.ErrTaskNotFound
		}
		return nil
	})
}

func (r *taskRepository) Release(ctx context.Context, scope coretenant.Scope, id string, next time.Time) error {
	return r.updateActive(ctx, scope, id,
		`status = 'pending', locked_until = NULL, next_attempt_at = $3`, next.UTC())
}

func (r *taskRepository) Complete(ctx context.Context, scope coretenant.Scope, id string) error {
	return r.updateActive(ctx, scope, id, `status = 'done', locked_until = NULL, finished_at = now()`)
}

func (r *taskRepository) RetryLater(ctx context.Context, scope coretenant.Scope, id string, next time.Time, code, msg string) error {
	return r.updateActive(ctx, scope, id,
		`status = 'pending', locked_until = NULL, attempts = attempts + 1, next_attempt_at = $3, last_error_code = $4, last_error = $5`,
		next.UTC(), code, msg)
}

func (r *taskRepository) Fail(ctx context.Context, scope coretenant.Scope, id, code, msg string) error {
	return r.updateActive(ctx, scope, id,
		`status = 'failed', locked_until = NULL, finished_at = now(), last_error_code = $3, last_error = $4`, code, msg)
}

func (r *taskRepository) Cancel(ctx context.Context, scope coretenant.Scope, id, reason string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE mikrotik_isolation_tasks
			SET status = 'cancelled', locked_until = NULL, finished_at = now(), last_error = $3
			WHERE organization_id = $1 AND id = $2 AND status IN ('pending', 'running')`,
			scope.OrganizationID(), id, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM mikrotik_isolation_tasks WHERE organization_id = $1 AND id = $2)`,
			scope.OrganizationID(), id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return mikrotik.ErrTaskNotFound
		}
		return nil // already finished: nothing to cancel
	})
}

func (r *taskRepository) markNotified(ctx context.Context, scope coretenant.Scope, id, column string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE mikrotik_isolation_tasks SET `+column+` = now()
			WHERE organization_id = $1 AND id = $2`, scope.OrganizationID(), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return mikrotik.ErrTaskNotFound
		}
		return nil
	})
}

func (r *taskRepository) MarkTenantNotified(ctx context.Context, scope coretenant.Scope, id string) error {
	return r.markNotified(ctx, scope, id, "tenant_notified_at")
}

func (r *taskRepository) MarkCustomerNotified(ctx context.Context, scope coretenant.Scope, id string) error {
	return r.markNotified(ctx, scope, id, "customer_notified_at")
}

func (r *taskRepository) Retry(ctx context.Context, scope coretenant.Scope, id string) (domain.IsolationTask, error) {
	var out domain.IsolationTask
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanTask(tx.QueryRow(ctx, `
			UPDATE mikrotik_isolation_tasks
			SET status = 'pending', attempts = 0, next_attempt_at = now(), locked_until = NULL,
			    finished_at = NULL, last_error = '', last_error_code = ''
			WHERE organization_id = $1 AND id = $2 AND status = 'failed'
			RETURNING `+taskColumns, scope.OrganizationID(), id))
		if isUniqueViolation(scanErr, "uq_mikrotik_isolation_tasks_active_secret") {
			return mikrotik.ErrTaskActiveExists
		}
		if !errors.Is(scanErr, pgx.ErrNoRows) {
			return scanErr
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM mikrotik_isolation_tasks WHERE organization_id = $1 AND id = $2)`,
			scope.OrganizationID(), id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return mikrotik.ErrTaskNotFound
		}
		return mikrotik.ErrTaskNotRetryable
	})
	if err != nil {
		return domain.IsolationTask{}, err
	}
	return out, nil
}

func (r *taskRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.IsolationTask, error) {
	var out domain.IsolationTask
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var scanErr error
		out, scanErr = scanTask(tx.QueryRow(ctx,
			`SELECT `+taskColumns+` FROM mikrotik_isolation_tasks WHERE organization_id = $1 AND id = $2`,
			scope.OrganizationID(), id))
		return scanErr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IsolationTask{}, mikrotik.ErrTaskNotFound
	}
	return out, err
}

func (r *taskRepository) List(ctx context.Context, scope coretenant.Scope, f TaskFilter, limit, offset int) ([]domain.TaskView, int64, error) {
	limit = clampLimit(limit)
	if offset < 0 {
		offset = 0
	}
	where := []string{"t.organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)))
	}
	if f.RouterID != "" {
		args = append(args, f.RouterID)
		where = append(where, fmt.Sprintf("s.router_id = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")
	const from = `
		FROM mikrotik_isolation_tasks t
		JOIN mikrotik_ppp_secrets s ON s.organization_id = t.organization_id AND s.id = t.secret_id
		JOIN mikrotik_routers r ON r.organization_id = s.organization_id AND r.id = s.router_id
		LEFT JOIN receivable_accounts a ON a.organization_id = s.organization_id AND a.id = s.account_id`

	items := []domain.TaskView{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) `+from+` WHERE `+cond, args...).Scan(&total); err != nil {
			return err
		}
		pageArgs := append(append([]interface{}{}, args...), limit, offset)
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT `+prefixed("t", taskColumns)+`,
				s.router_id, r.name, s.name, s.account_id, a.name `+from+` WHERE `+cond+`
			ORDER BY t.created_at DESC, t.id LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2), pageArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.TaskView
			var accountID, accountName *string
			t := &v.IsolationTask
			if err := rows.Scan(&t.ID, &t.OrganizationID, &t.SecretID, &t.Action, &t.Priority, &t.Status, &t.Attempts,
				&t.NextAttemptAt, &t.LockedUntil, &t.LastError, &t.LastErrorCode, &t.TenantNotifiedAt, &t.CustomerNotifiedAt,
				&t.CreatedByEvent, &t.CreatedAt, &t.StartedAt, &t.FinishedAt,
				&v.RouterID, &v.RouterName, &v.SecretName, &accountID, &accountName); err != nil {
				return err
			}
			v.AccountID, v.AccountName = derefString(accountID), derefString(accountName)
			items = append(items, v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *taskRepository) CountByStatus(ctx context.Context, scope coretenant.Scope) (map[string]int, error) {
	counts := map[string]int{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT status, COUNT(*) FROM mikrotik_isolation_tasks
			WHERE organization_id = $1 GROUP BY status`, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var status string
			var n int
			if err := rows.Scan(&status, &n); err != nil {
				return err
			}
			counts[status] = n
		}
		return rows.Err()
	})
	return counts, err
}
