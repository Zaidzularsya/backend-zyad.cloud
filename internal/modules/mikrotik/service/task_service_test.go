package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

type fakeTaskStore struct {
	retried   domain.IsolationTask
	retryErr  error
	counts    map[string]int
	oldest    *time.Time
	retryCall int
}

func (f *fakeTaskStore) List(context.Context, coretenant.Scope, repository.TaskFilter, int, int) ([]domain.TaskView, int64, error) {
	return nil, 0, nil
}
func (f *fakeTaskStore) CountByStatus(context.Context, coretenant.Scope) (map[string]int, error) {
	return f.counts, nil
}
func (f *fakeTaskStore) OldestPendingRestore(context.Context, coretenant.Scope) (*time.Time, error) {
	return f.oldest, nil
}
func (f *fakeTaskStore) Retry(context.Context, coretenant.Scope, string) (domain.IsolationTask, error) {
	f.retryCall++
	return f.retried, f.retryErr
}

type fakeTaskSecrets struct {
	repository.SecretRepository
}

func (fakeTaskSecrets) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.PPPSecret, error) {
	return domain.PPPSecret{ID: id, RouterID: "router-1", Name: "alice"}, nil
}

type captureAuditor struct{ entries []repository.AuditEntry }

func (a *captureAuditor) InsertAudit(_ context.Context, _ coretenant.Scope, e repository.AuditEntry) error {
	a.entries = append(a.entries, e)
	return nil
}

func TestTaskService_Retry_WritesAudit(t *testing.T) {
	store := &fakeTaskStore{retried: domain.IsolationTask{ID: "t1", SecretID: "s1", Action: domain.ActionRestore, Status: domain.TaskPending}}
	audit := &captureAuditor{}
	svc := NewTaskService(store, fakeTaskSecrets{}, audit)
	view, err := svc.Retry(context.Background(), coretenant.Scope{}, "t1", "user-1")
	if err != nil || view.Status != domain.TaskPending || view.RouterID != "router-1" {
		t.Fatalf("retry: %+v %v", view, err)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d", len(audit.entries))
	}
	e := audit.entries[0]
	if e.Event != "task.retried" || e.ActorUserID != "user-1" || e.TargetID != "router-1" ||
		e.Metadata["target"] != "task" || e.Metadata["task_id"] != "t1" || e.Metadata["secret_id"] != "s1" || e.Metadata["action"] != domain.ActionRestore {
		t.Fatalf("audit = %+v", e)
	}
}

func TestTaskService_Retry_ErrorWritesNoAudit(t *testing.T) {
	store := &fakeTaskStore{retryErr: mikrotik.ErrTaskNotRetryable}
	audit := &captureAuditor{}
	_, err := NewTaskService(store, fakeTaskSecrets{}, audit).Retry(context.Background(), coretenant.Scope{}, "t1", "u")
	if !errors.Is(err, mikrotik.ErrTaskNotRetryable) || len(audit.entries) != 0 {
		t.Fatalf("err=%v audit=%d", err, len(audit.entries))
	}
}

func TestTaskService_Summary_ZeroFilled(t *testing.T) {
	oldest := time.Now().Add(-time.Hour)
	store := &fakeTaskStore{counts: map[string]int{domain.TaskFailed: 2}, oldest: &oldest}
	sum, err := NewTaskService(store, fakeTaskSecrets{}, &captureAuditor{}).Summary(context.Background(), coretenant.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{domain.TaskPending, domain.TaskRunning, domain.TaskDone, domain.TaskFailed, domain.TaskCancelled} {
		if _, ok := sum.Counts[st]; !ok {
			t.Errorf("status %s missing", st)
		}
	}
	if sum.Counts[domain.TaskFailed] != 2 || sum.Counts[domain.TaskPending] != 0 || sum.OldestPendingRestoreAt == nil {
		t.Fatalf("summary = %+v", sum)
	}
}
