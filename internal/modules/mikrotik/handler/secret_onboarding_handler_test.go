package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

const onboardSecretID = "22222222-2222-2222-2222-222222222222"

type fakeSecretOnboarding struct {
	in       service.OnboardInput
	actor    string
	runErr   error
	state    service.OnboardState
	hasState bool
}

func (f *fakeSecretOnboarding) Preview(_ context.Context, _ coretenant.Scope, in service.OnboardInput, actor string) (service.OnboardPlan, error) {
	f.in, f.actor = in, actor
	return service.OnboardPlan{SecretName: "budi", ContactAction: "new", ContractStartDate: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
		DuplicateOf: &service.OnboardContactRef{ID: "c-old", Name: "Budi"}}, nil
}

func (f *fakeSecretOnboarding) Run(_ context.Context, _ coretenant.Scope, in service.OnboardInput, actor string) (service.OnboardState, error) {
	f.in, f.actor = in, actor
	return f.state, f.runErr
}

func (f *fakeSecretOnboarding) State(context.Context, coretenant.Scope, string) (service.OnboardState, bool, error) {
	return f.state, f.hasState, nil
}

func onboardRouter(t *testing.T, svc *fakeSecretOnboarding) http.Handler {
	r := newTestRouter(t)
	h := NewSecretOnboardingHandler(svc)
	r.POST("/secrets/:id/onboard/preview", h.Preview)
	r.POST("/secrets/:id/onboard", h.Run)
	r.GET("/secrets/:id/onboard", h.Get)
	return r
}

const onboardBody = `{"customer":{"new":{"name":"Sari","phone":"0899","email":"s@b.com"}},"product_id":"p1",
	"quantity":"2","unit_price":"150000","billing_day":25,"create_initial_invoice":false,"channels":["email"]}`

func TestOnboardPreview_BindsInputAndReturnsPlan(t *testing.T) {
	svc := &fakeSecretOnboarding{}
	w := performJSON(onboardRouter(t, svc), http.MethodPost, "/secrets/"+onboardSecretID+"/onboard/preview", onboardBody)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	in := svc.in
	if in.SecretID != onboardSecretID || in.Customer.New == nil || in.Customer.New.Name != "Sari" || in.ProductID != "p1" ||
		in.Quantity != "2" || in.UnitPrice != "150000" || in.BillingDay != 25 || in.CreateInitialInvoice || len(in.Channels) != 1 {
		t.Fatalf("input = %+v", in)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"contract_start_date":"2026-10-25"`) || !strings.Contains(body, `"duplicate_of":{"id":"c-old"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestOnboardRun_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&service.OnboardDuplicateError{Existing: service.OnboardContactRef{ID: "c-old"}}, http.StatusConflict, "CONTACT_DUPLICATE"},
		{mikrotik.ErrOnboardingInProgress, http.StatusConflict, "MIKROTIK_ONBOARDING_IN_PROGRESS"},
		{fmt.Errorf("%w: sales_order.confirm", mikrotik.ErrOnboardingForbidden), http.StatusForbidden, "MIKROTIK_ONBOARDING_FORBIDDEN"},
		{mikrotik.ErrSecretHasContract, http.StatusConflict, "MIKROTIK_SECRET_HAS_CONTRACT"},
		{mikrotik.ErrContactNotFound, http.StatusNotFound, "CONTACT_NOT_FOUND"},
		{&mikrotik.InvalidInputError{Message: "Tanggal tagih"}, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
	}
	for _, c := range cases {
		svc := &fakeSecretOnboarding{runErr: c.err, state: service.OnboardState{Stage: service.StageDeal, LastError: "x"}}
		w := performJSON(onboardRouter(t, svc), http.MethodPost, "/secrets/"+onboardSecretID+"/onboard", onboardBody)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != c.status || body.Code != c.code {
			t.Errorf("%v → %d %s, want %d %s", c.err, w.Code, w.Body.String(), c.status, c.code)
		}
	}
	// Forbidden names the missing permission.
	svc := &fakeSecretOnboarding{runErr: fmt.Errorf("%w: sales_order.confirm", mikrotik.ErrOnboardingForbidden)}
	w := performJSON(onboardRouter(t, svc), http.MethodPost, "/secrets/"+onboardSecretID+"/onboard", onboardBody)
	if !strings.Contains(w.Body.String(), "sales_order.confirm") {
		t.Fatalf("forbidden body = %s", w.Body.String())
	}
}

func TestOnboardRun_SuccessAndGetState(t *testing.T) {
	svc := &fakeSecretOnboarding{state: service.OnboardState{SecretID: onboardSecretID, Stage: service.StagePaired, ContractID: "ctr1"}, hasState: true}
	r := onboardRouter(t, svc)
	w := performJSON(r, http.MethodPost, "/secrets/"+onboardSecretID+"/onboard", onboardBody)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"stage":"paired"`) {
		t.Fatalf("run = %d %s", w.Code, w.Body.String())
	}
	w = performJSON(r, http.MethodGet, "/secrets/"+onboardSecretID+"/onboard", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"contract_id":"ctr1"`) {
		t.Fatalf("get = %d %s", w.Code, w.Body.String())
	}
	svc.hasState = false
	w = performJSON(r, http.MethodGet, "/secrets/"+onboardSecretID+"/onboard", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data":null`) && !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("get none = %d %s", w.Code, w.Body.String())
	}
	if w := performJSON(r, http.MethodPost, "/secrets/not-a-uuid/onboard", onboardBody); w.Code != http.StatusNotFound {
		t.Fatalf("bad id → %d", w.Code)
	}
}

func (f *fakeSecretOnboarding) Detail(context.Context, coretenant.Scope, string) (service.SecretDetail, error) {
	acc := "acct1"
	return service.SecretDetail{
		Secret:     domain.PPPSecret{ID: onboardSecretID, Name: "budi", Disabled: true, Present: true, AccountID: &acc, AccountName: "Budi"},
		Onboarding: &service.OnboardState{Stage: service.StagePaired},
		Contract:   &service.ContractRef{ID: "ctr1", Number: "CTR-2026-0001"},
	}, nil
}

func TestSecretDetailRoute(t *testing.T) {
	r := newTestRouter(t)
	r.GET("/secrets/:id", NewSecretOnboardingHandler(&fakeSecretOnboarding{}).Detail)
	w := performJSON(r, http.MethodGet, "/secrets/"+onboardSecretID, "")
	body := w.Body.String()
	for _, want := range []string{`"name":"budi"`, `"disabled":true`, `"account_name":"Budi"`, `"stage":"paired"`, `"number":"CTR-2026-0001"`, `"can_onboard":false`} {
		if !strings.Contains(body, want) {
			t.Fatalf("detail %d missing %s: %s", w.Code, want, body)
		}
	}
}
