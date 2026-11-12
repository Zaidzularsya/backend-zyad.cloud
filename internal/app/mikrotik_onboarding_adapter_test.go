package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

// ---- fakes ----

type obQuotations struct {
	status  map[string]crmdomain.QuotationStatus
	calls   []string
	created crmservice.CreateQuotationInput
}

func (q *obQuotations) Create(_ context.Context, _ coretenant.Scope, in crmservice.CreateQuotationInput) (crmdomain.Quotation, error) {
	q.created = in
	q.status["q-new"] = crmdomain.QuotationStatusDraft
	return crmdomain.Quotation{ID: "q-new"}, nil
}
func (q *obQuotations) Get(_ context.Context, _ coretenant.Scope, id string) (crmdomain.Quotation, error) {
	return crmdomain.Quotation{ID: id, Status: q.status[id]}, nil
}
func (q *obQuotations) Send(_ context.Context, _ coretenant.Scope, id, _ string) (crmdomain.Quotation, error) {
	q.calls = append(q.calls, "send")
	q.status[id] = crmdomain.QuotationStatusSent
	return crmdomain.Quotation{ID: id}, nil
}
func (q *obQuotations) Approve(_ context.Context, _ coretenant.Scope, id, _ string) (crmdomain.Quotation, error) {
	q.calls = append(q.calls, "approve")
	q.status[id] = crmdomain.QuotationStatusApproved
	return crmdomain.Quotation{ID: id}, nil
}

type obSalesOrders struct {
	so      crmdomain.SalesOrder
	draft   crmrepo.SalesOrderDraftFields
	calls   []string
	billing crmdomain.BillingStatus
	created bool
}

func (s *obSalesOrders) QuotationApproved(context.Context, coretenant.Scope, crmdomain.Quotation, string) error {
	s.calls = append(s.calls, "hook")
	s.created = true
	return nil
}
func (s *obSalesOrders) FindByQuotation(context.Context, coretenant.Scope, string) (crmdomain.SalesOrder, error) {
	if !s.created {
		return crmdomain.SalesOrder{}, pgx.ErrNoRows
	}
	return crmdomain.SalesOrder{ID: "so1"}, nil
}
func (s *obSalesOrders) Get(context.Context, coretenant.Scope, string) (crmdomain.SalesOrder, error) {
	return s.so, nil
}
func (s *obSalesOrders) UpdateDraft(_ context.Context, _ coretenant.Scope, _ string, f crmrepo.SalesOrderDraftFields, _ string) (crmdomain.SalesOrder, error) {
	s.calls = append(s.calls, "update_draft")
	s.draft = f
	return s.so, nil
}
func (s *obSalesOrders) bill() crmdomain.SalesOrder {
	so := s.so
	so.Status, so.BillingStatus = crmdomain.SalesOrderConfirmed, s.billing
	if s.billing == crmdomain.BillingDone {
		so.ReceivableAccountID, so.ContractID = "acct1", "ctr1"
	} else {
		so.BillingError = "Kuota invoice penuh."
	}
	return so
}
func (s *obSalesOrders) Confirm(context.Context, coretenant.Scope, string, string) (crmdomain.SalesOrder, error) {
	s.calls = append(s.calls, "confirm")
	return s.bill(), nil
}
func (s *obSalesOrders) RetryBilling(context.Context, coretenant.Scope, string, string) (crmdomain.SalesOrder, error) {
	s.calls = append(s.calls, "retry")
	return s.bill(), nil
}

type obPipelines struct{ list []crmdomain.Pipeline }

func (p obPipelines) List(context.Context, coretenant.Scope, crmrepo.PipelineListFilter) ([]crmdomain.Pipeline, int64, error) {
	return p.list, int64(len(p.list)), nil
}

type obDeals struct{ got crmrepo.CreateDealParams }

func (d *obDeals) Create(_ context.Context, _ coretenant.Scope, p crmrepo.CreateDealParams) (crmdomain.Deal, error) {
	d.got = p
	return crmdomain.Deal{ID: "d1"}, nil
}

func obScope(t *testing.T) coretenant.Scope {
	t.Helper()
	scope, err := coretenant.NewScope(mikrotikTenantContext(t, rtOrg))
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// ---- adapter tests ----

func TestOnboardingCRM_ApproveQuotationWalksStatuses(t *testing.T) {
	q := &obQuotations{status: map[string]crmdomain.QuotationStatus{
		"draft": crmdomain.QuotationStatusDraft, "approved": crmdomain.QuotationStatusApproved, "rejected": crmdomain.QuotationStatusRejected,
	}}
	a := mikrotikOnboardingCRM{quotations: q}
	if err := a.ApproveQuotation(context.Background(), obScope(t), "draft", "u1"); err != nil || strings.Join(q.calls, ",") != "send,approve" {
		t.Fatalf("draft: calls=%v err=%v", q.calls, err)
	}
	q.calls = nil
	if err := a.ApproveQuotation(context.Background(), obScope(t), "approved", "u1"); err != nil || len(q.calls) != 0 {
		t.Fatalf("approved must be a no-op: calls=%v err=%v", q.calls, err)
	}
	var appErr *coreerrors.AppError
	if err := a.ApproveQuotation(context.Background(), obScope(t), "rejected", "u1"); !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("rejected: err=%v", err)
	}
}

func TestOnboardingCRM_EnsureSalesOrderRunsHookWhenMissing(t *testing.T) {
	so := &obSalesOrders{}
	a := mikrotikOnboardingCRM{quotations: &obQuotations{status: map[string]crmdomain.QuotationStatus{}}, salesOrders: so, soFinder: so}
	id, err := a.EnsureSalesOrder(context.Background(), obScope(t), "q1", "u1")
	if err != nil || id != "so1" || strings.Join(so.calls, ",") != "hook" {
		t.Fatalf("id=%q calls=%v err=%v", id, so.calls, err)
	}
	so.calls = nil
	if _, err := a.EnsureSalesOrder(context.Background(), obScope(t), "q1", "u1"); err != nil || len(so.calls) != 0 {
		t.Fatalf("existing SO must not rerun the hook: %v %v", so.calls, err)
	}
}

func TestOnboardingCRM_ConfirmSalesOrder(t *testing.T) {
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	so := &obSalesOrders{so: crmdomain.SalesOrder{ID: "so1", Status: crmdomain.SalesOrderDraft, BillToName: "Budi", BillToEmail: "b@b.com", PICUserID: "u-pic"},
		billing: crmdomain.BillingDone}
	a := mikrotikOnboardingCRM{salesOrders: so}
	out, err := a.ConfirmSalesOrder(context.Background(), obScope(t), "so1", mikrotikservice.ConfirmOrder{StartDate: start, SkipInitialInvoice: true, Channels: []string{"email"}}, "u1")
	if err != nil || out.ContractID != "ctr1" || out.AccountID != "acct1" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	d := so.draft
	if d.SkipInitialInvoice == nil || !*d.SkipInitialInvoice || !d.StartDate.Equal(start) || d.BillToName != "Budi" || d.PICUserID != "u-pic" || d.Channels[0] != "email" {
		t.Fatalf("draft fields = %+v", d)
	}

	// Billing failed: the step fails with the SO's message; a retry uses RetryBilling.
	so.billing = crmdomain.BillingFailed
	so.calls = nil
	_, err = a.ConfirmSalesOrder(context.Background(), obScope(t), "so1", mikrotikservice.ConfirmOrder{StartDate: start}, "u1")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Message != "Kuota invoice penuh." {
		t.Fatalf("billing failure err = %v", err)
	}
	so.so.Status, so.so.BillingStatus = crmdomain.SalesOrderConfirmed, crmdomain.BillingFailed
	so.billing = crmdomain.BillingDone
	so.calls = nil
	if out, err := a.ConfirmSalesOrder(context.Background(), obScope(t), "so1", mikrotikservice.ConfirmOrder{StartDate: start}, "u1"); err != nil || out.ContractID != "ctr1" || strings.Join(so.calls, ",") != "retry" {
		t.Fatalf("retry: out=%+v calls=%v err=%v", out, so.calls, err)
	}
}

func TestOnboardingCRM_CreateDealUsesDefaultPipelineFirstOpenStage(t *testing.T) {
	archived := time.Now()
	deals := &obDeals{}
	a := mikrotikOnboardingCRM{deals: deals, pipelines: obPipelines{list: []crmdomain.Pipeline{
		{ID: "p-new", Stages: []crmdomain.PipelineStage{{ID: "x", Position: 0}}},
		{ID: "p-def", IsDefault: true, Stages: []crmdomain.PipelineStage{
			{ID: "won", Position: 0, IsWon: true}, {ID: "s2", Position: 2}, {ID: "s1", Position: 1},
		}},
		{ID: "p-arch", IsDefault: true, ArchivedAt: &archived},
	}}}
	if _, err := a.CreateDeal(context.Background(), obScope(t), mikrotikservice.OnboardDeal{ContactID: "c1", Title: "T", Value: "1.00"}, "u1"); err != nil {
		t.Fatal(err)
	}
	if g := deals.got; g.PipelineID != "p-def" || g.StageID != "s1" || g.Source != crmdomain.DealSourcePPPoEImport || g.ContactID != "c1" || g.OwnerUserID != "u1" {
		t.Fatalf("deal params = %+v", g)
	}
	if mikrotikservice.OnboardingDealSource != crmdomain.DealSourcePPPoEImport {
		t.Fatal("mikrotik and crm deal source constants diverged")
	}
	a.pipelines = obPipelines{}
	var appErr *coreerrors.AppError
	if _, err := a.CreateDeal(context.Background(), obScope(t), mikrotikservice.OnboardDeal{}, "u1"); !errors.As(err, &appErr) || appErr.Code != "CRM_PIPELINE_REQUIRED" {
		t.Fatalf("no pipeline err = %v", err)
	}
}

func TestOnboardingPermissions_ListsMissing(t *testing.T) {
	p := mikrotikOnboardingPermissions{checker: permissionSetChecker{allowed: map[string]bool{
		"deal.create": true, "quotation.create": true, "quotation.send": true, "quotation.approve": true, "sales_order.manage": true,
	}}}
	missing, err := p.Missing(context.Background(), obScope(t), "u1", true)
	if err != nil || strings.Join(missing, ",") != "contact.create,sales_order.confirm" {
		t.Fatalf("missing = %v err=%v", missing, err)
	}
	missing, _ = p.Missing(context.Background(), obScope(t), "u1", false)
	if strings.Join(missing, ",") != "sales_order.confirm" {
		t.Fatalf("existing contact: missing = %v", missing)
	}
}

// ---- routes ----

type countingOnboarding struct{ calls int }

func (c *countingOnboarding) Preview(context.Context, coretenant.Scope, mikrotikservice.OnboardInput, string) (mikrotikservice.OnboardPlan, error) {
	c.calls++
	return mikrotikservice.OnboardPlan{}, nil
}
func (c *countingOnboarding) Run(context.Context, coretenant.Scope, mikrotikservice.OnboardInput, string) (mikrotikservice.OnboardState, error) {
	c.calls++
	return mikrotikservice.OnboardState{}, nil
}
func (c *countingOnboarding) State(context.Context, coretenant.Scope, string) (mikrotikservice.OnboardState, bool, error) {
	c.calls++
	return mikrotikservice.OnboardState{}, false, nil
}

func TestMikrotikOnboardRoutes_Guards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct{ method, path, perm string }{
		{http.MethodPost, "/secrets/" + rtSecretID(t) + "/onboard/preview", "mikrotik_secret.pair"},
		{http.MethodPost, "/secrets/" + rtSecretID(t) + "/onboard", "mikrotik_secret.pair"},
		{http.MethodGet, "/secrets/" + rtSecretID(t) + "/onboard", "mikrotik_secret.read"},
	}
	run := func(entitled bool, granted []string, method, path string) (int, int) {
		svc := &countingOnboarding{}
		allowed := map[string]bool{}
		for _, p := range granted {
			allowed[p] = true
		}
		engine := gin.New()
		tc := mikrotikTenantContext(t, rtOrg)
		protected := engine.Group("/api/v1")
		protected.Use(middleware.Authenticate(fakeBearerAuthenticator{}), func(c *gin.Context) {
			c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
			c.Next()
		})
		registerMikrotikRoutes(protected, Dependencies{
			CRMEntitlementChecker:           &recordingEntitlements{allow: entitled},
			PermissionChecker:               permissionSetChecker{allowed: allowed},
			MikrotikRouterHandler:           mikrotikhandler.NewRouterHandler(&countingRouterService{}),
			MikrotikSecretOnboardingHandler: mikrotikhandler.NewSecretOnboardingHandler(svc),
		})
		req := httptest.NewRequest(method, "/api/v1/app/mikrotik"+path, strings.NewReader(`{"product_id":"p1","billing_day":5}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code, svc.calls
	}
	for _, c := range cases {
		if code, calls := run(false, []string{c.perm}, c.method, c.path); code != http.StatusForbidden || calls != 0 {
			t.Errorf("%s %s without entitlement: %d/%d", c.method, c.path, code, calls)
		}
		if code, calls := run(true, nil, c.method, c.path); code != http.StatusForbidden || calls != 0 {
			t.Errorf("%s %s without %s: %d/%d", c.method, c.path, c.perm, code, calls)
		}
		if code, calls := run(true, []string{c.perm}, c.method, c.path); code >= 300 || calls != 1 {
			t.Errorf("%s %s with %s: %d/%d", c.method, c.path, c.perm, code, calls)
		}
	}
}

func rtSecretID(t *testing.T) string {
	t.Helper()
	return "33333333-3333-3333-3333-333333333333"
}
