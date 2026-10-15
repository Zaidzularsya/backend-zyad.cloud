package handler

import (
	"context"
	"net/http"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/playbook"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
)

type fakeActivityService struct {
	service.ActivityService
	completeErr    error
	completeParams repository.CompleteActivityParams
}

func (f *fakeActivityService) CompleteWithOutcome(_ context.Context, _ coretenant.Scope, _ string, p repository.CompleteActivityParams) (repository.CompleteActivityResult, error) {
	f.completeParams = p
	return repository.CompleteActivityResult{}, f.completeErr
}

func completeRouter(t *testing.T, svc service.ActivityService) http.Handler {
	r := newTestRouter(t)
	r.POST("/activities/:id/complete", NewActivityHandler(svc).Complete)
	return r
}

func TestCompleteActivityMapsOutcomeErrors(t *testing.T) {
	cases := map[error]int{
		playbook.ErrOutcomeRequired:        http.StatusUnprocessableEntity,
		playbook.ErrOutcomeInvalid:         http.StatusUnprocessableEntity,
		playbook.ErrOutcomeInputRequired:   http.StatusUnprocessableEntity,
		repository.ErrPlaybookRunNotActive: http.StatusNotFound,
	}
	for svcErr, want := range cases {
		svc := &fakeActivityService{completeErr: svcErr}
		w := performJSON(completeRouter(t, svc), http.MethodPost, "/activities/a1/complete", `{"outcome_key":""}`)
		if w.Code != want {
			t.Fatalf("%v → %d, want %d (%s)", svcErr, w.Code, want, w.Body.String())
		}
	}
}

func TestCompleteActivityParsesInput(t *testing.T) {
	svc := &fakeActivityService{}
	router := completeRouter(t, svc)
	w := performJSON(router, http.MethodPost, "/activities/a1/complete",
		`{"outcome_key":"qualified","requirements":{"summary":"CRM","budget_estimate":"1000.50","target_date":"2026-12-01"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	got := svc.completeParams
	if got.OutcomeKey != "qualified" || got.Input.Requirements.Summary != "CRM" || *got.Input.Requirements.BudgetEstimate != "1000.50" {
		t.Fatalf("%+v", got)
	}
	w = performJSON(router, http.MethodPost, "/activities/a1/complete", `{"outcome_key":"x","reschedule_at":"besok"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad reschedule_at → %d", w.Code)
	}
	w = performJSON(router, http.MethodPost, "/activities/a1/complete", `{"outcome_key":"qualified","requirements":{"summary":"x","budget_estimate":"-5"}}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad budget → %d", w.Code)
	}
}
