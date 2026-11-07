package service

import (
	"context"
	"log/slog"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// TaskStore is the subset of repository.TaskRepository the dashboard uses.
type TaskStore interface {
	List(ctx context.Context, scope coretenant.Scope, f repository.TaskFilter, limit, offset int) ([]domain.TaskView, int64, error)
	CountByStatus(ctx context.Context, scope coretenant.Scope) (map[string]int, error)
	OldestPendingRestore(ctx context.Context, scope coretenant.Scope) (*time.Time, error)
	Retry(ctx context.Context, scope coretenant.Scope, id string) (domain.IsolationTask, error)
}

// TaskSecretReader resolves the router a task's secret lives on (audit target).
type TaskSecretReader interface {
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.PPPSecret, error)
}

// TaskSummary is the dashboard header: one count per status (all statuses
// present) and the age of the longest-waiting restore.
type TaskSummary struct {
	Counts                 map[string]int
	OldestPendingRestoreAt *time.Time
}

// TaskService backs the isolation task dashboard API.
type TaskService struct {
	tasks   TaskStore
	secrets TaskSecretReader
	audit   Auditor
}

func NewTaskService(tasks TaskStore, secrets TaskSecretReader, audit Auditor) *TaskService {
	return &TaskService{tasks: tasks, secrets: secrets, audit: audit}
}

// List pages tasks newest first; the repository clamps limit/offset.
func (s *TaskService) List(ctx context.Context, scope coretenant.Scope, f repository.TaskFilter, limit, offset int) ([]domain.TaskView, int64, error) {
	return s.tasks.List(ctx, scope, f, limit, offset)
}

func (s *TaskService) Summary(ctx context.Context, scope coretenant.Scope) (TaskSummary, error) {
	raw, err := s.tasks.CountByStatus(ctx, scope)
	if err != nil {
		return TaskSummary{}, err
	}
	counts := map[string]int{
		domain.TaskPending: 0, domain.TaskRunning: 0, domain.TaskDone: 0, domain.TaskFailed: 0, domain.TaskCancelled: 0,
	}
	for status, n := range raw {
		counts[status] = n
	}
	oldest, err := s.tasks.OldestPendingRestore(ctx, scope)
	if err != nil {
		return TaskSummary{}, err
	}
	return TaskSummary{Counts: counts, OldestPendingRestoreAt: oldest}, nil
}

// Retry puts a failed task back in the queue and audits it. The returned view
// carries the router id and secret name only (no router name join).
func (s *TaskService) Retry(ctx context.Context, scope coretenant.Scope, id, actor string) (domain.TaskView, error) {
	task, err := s.tasks.Retry(ctx, scope, id)
	if err != nil {
		return domain.TaskView{}, err
	}
	view := domain.TaskView{IsolationTask: task}
	target := task.SecretID
	if secret, err := s.secrets.FindByID(ctx, scope, task.SecretID); err == nil {
		view.RouterID, view.SecretName = secret.RouterID, secret.Name
		target = secret.RouterID
	}
	// Audit after the mutation committed; a failed audit row must not fail it.
	if err := s.audit.InsertAudit(ctx, scope, repository.AuditEntry{
		Event: "task.retried", ActorUserID: actor, TargetID: target,
		Metadata: map[string]any{"target": "task", "task_id": task.ID, "secret_id": task.SecretID, "action": task.Action},
	}); err != nil {
		slog.Warn("mikrotik: audit insert failed", "task_id", task.ID, "event", "task.retried", "error", scrub(err.Error()))
	}
	return view, nil
}
