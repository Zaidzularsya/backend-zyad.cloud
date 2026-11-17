package service

import (
	"context"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

type stageOverdue struct {
	fakePublicOverdue
	accountOf map[string]string
}

func (f stageOverdue) AccountOfInvoice(_ context.Context, _ coretenant.Scope, id string) (string, error) {
	a, ok := f.accountOf[id]
	if !ok {
		return "", mikrotik.ErrAccountNotFound
	}
	return a, nil
}

type fakeRestoreTasks struct {
	bySecret map[string]domain.IsolationTask
}

func (f fakeRestoreTasks) LatestRestoreBySecret(_ context.Context, _ coretenant.Scope, ids []string) (map[string]domain.IsolationTask, error) {
	out := map[string]domain.IsolationTask{}
	for _, id := range ids {
		if t, ok := f.bySecret[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

var stageNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func stageSec(id, state string) domain.PPPSecret {
	return domain.PPPSecret{ID: id, Name: id, IsolationState: state}
}

func stageTask(status string, age time.Duration) domain.IsolationTask {
	return domain.IsolationTask{Action: domain.ActionRestore, Status: status, CreatedAt: stageNow.Add(-age)}
}

func TestStage(t *testing.T) {
	iso, norm := domain.IsolationIsolated, domain.IsolationNormal
	other := []TriggerInvoice{{ID: "i9"}}
	// due 3 days ago: a candidate only while grace < 3.
	recent := []TriggerInvoice{{ID: "i8", DueDate: pubToday.AddDate(0, 0, -3)}}
	exempt := &domain.CustomerPolicy{AccountID: "a1", Exempt: true}
	cases := []struct {
		name       string
		secrets    []domain.PPPSecret
		candidates []TriggerInvoice
		tasks      map[string]domain.IsolationTask
		paidAgo    time.Duration
		unknownInv bool
		grace      int
		policy     *domain.CustomerPolicy
		want       Stage
		applicable bool
	}{
		{name: "no secrets", secrets: nil, paidAgo: time.Minute, applicable: false},
		{name: "unknown invoice", unknownInv: true, paidAgo: time.Minute, applicable: false},
		{name: "all normal", secrets: []domain.PPPSecret{stageSec("s1", norm), stageSec("s2", norm)}, paidAgo: time.Minute, want: StageActive, applicable: true},
		{name: "isolated no task fresh", secrets: []domain.PPPSecret{stageSec("s1", iso)}, paidAgo: 9 * time.Minute, want: StageReceived, applicable: true},
		{name: "mode off: isolated no task 11m", secrets: []domain.PPPSecret{stageSec("s1", iso)}, paidAgo: 11 * time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "no task exactly 10m", secrets: []domain.PPPSecret{stageSec("s1", iso)}, paidAgo: 10 * time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "pending young", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskPending, 2*time.Minute)}, paidAgo: 2 * time.Minute, want: StageRestoring, applicable: true},
		{name: "running young", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskRunning, time.Minute)}, paidAgo: 2 * time.Minute, want: StageRestoring, applicable: true},
		{name: "pending old", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskPending, 10*time.Minute)}, paidAgo: 12 * time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "failed immediately", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskFailed, time.Minute)}, paidAgo: time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "other invoices beats everything", secrets: []domain.PPPSecret{stageSec("s1", iso)}, candidates: other, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskPending, time.Minute)}, paidAgo: time.Minute, want: StageOtherInvoices, applicable: true},
		{name: "candidates but all normal", secrets: []domain.PPPSecret{stageSec("s1", norm)}, candidates: other, paidAgo: time.Minute, want: StageActive, applicable: true},
		{name: "multi: one isolated no task fresh", secrets: []domain.PPPSecret{stageSec("s1", norm), stageSec("s2", iso)}, paidAgo: time.Minute, want: StageReceived, applicable: true},
		{name: "multi: restoring + failed", secrets: []domain.PPPSecret{stageSec("s1", iso), stageSec("s2", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskRunning, time.Minute), "s2": stageTask(domain.TaskFailed, time.Minute)}, paidAgo: time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "other invoice within grace is not other_invoices", secrets: []domain.PPPSecret{stageSec("s1", iso)}, candidates: recent, grace: 7, paidAgo: time.Minute, want: StageReceived, applicable: true},
		{name: "other invoice past grace", secrets: []domain.PPPSecret{stageSec("s1", iso)}, candidates: recent, grace: 2, paidAgo: time.Minute, want: StageOtherInvoices, applicable: true},
		{name: "grace 0 as before", secrets: []domain.PPPSecret{stageSec("s1", iso)}, candidates: recent, paidAgo: time.Minute, want: StageOtherInvoices, applicable: true},
		{name: "exempt account never other_invoices", secrets: []domain.PPPSecret{stageSec("s1", iso)}, candidates: other, policy: exempt, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskPending, time.Minute)}, paidAgo: time.Minute, want: StageRestoring, applicable: true},
		{name: "restore done but still isolated, 5m since paid", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskDone, 4*time.Minute)}, paidAgo: 5 * time.Minute, want: StageReceived, applicable: true},
		{name: "cancelled and isolated, 11m since paid", secrets: []domain.PPPSecret{stageSec("s1", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskCancelled, 10*time.Minute)}, paidAgo: 11 * time.Minute, want: StageNeedsHelp, applicable: true},
		{name: "multi: restoring + received", secrets: []domain.PPPSecret{stageSec("s1", iso), stageSec("s2", iso)}, tasks: map[string]domain.IsolationTask{"s1": stageTask(domain.TaskRunning, time.Minute)}, paidAgo: time.Minute, want: StageRestoring, applicable: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewPublicService(PublicServiceDeps{
				Secrets: fakePublicSecrets{map[string][]domain.PPPSecret{"a1": c.secrets}},
				Overdue: stageOverdue{
					fakePublicOverdue: fakePublicOverdue{map[string][]TriggerInvoice{"a1": c.candidates}},
					accountOf: func() map[string]string {
						if c.unknownInv {
							return nil
						}
						return map[string]string{"inv": "a1"}
					}(),
				},
				Tasks:   fakeRestoreTasks{c.tasks},
				Control: fakePublicControl{grace: c.grace},
				Policy: func() fakePublicPolicy {
					if c.policy == nil {
						return fakePublicPolicy{}
					}
					return fakePublicPolicy{map[string]domain.CustomerPolicy{"a1": *c.policy}}
				}(),
			})
			got, ok, err := svc.Stage(context.Background(), coretenant.Scope{}, "inv", stageNow.Add(-c.paidAgo), stageNow, pubToday)
			if err != nil {
				t.Fatal(err)
			}
			if ok != c.applicable || got != c.want {
				t.Fatalf("got (%q, %v) want (%q, %v)", got, ok, c.want, c.applicable)
			}
		})
	}
}
