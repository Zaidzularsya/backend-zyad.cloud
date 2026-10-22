package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/organization"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

// quotaQuotationRepo records state-changing calls so tests can prove the
// quotation was not touched when the sales order quota is full.
type quotaQuotationRepo struct {
	fakeQuotationRepo
	approved, accepted int
}

func (r *quotaQuotationRepo) Approve(_ context.Context, _ coretenant.Scope, id, _ string) (domain.Quotation, error) {
	r.approved++
	return domain.Quotation{ID: id, Status: domain.QuotationStatusApproved}, nil
}

func (r *quotaQuotationRepo) AcceptSelfServe(_ context.Context, _ coretenant.Scope, id, _ string) (domain.Quotation, error) {
	r.accepted++
	return domain.Quotation{ID: id, Status: domain.QuotationStatusApproved}, nil
}

func quotationInput() CreateQuotationInput {
	return CreateQuotationInput{Items: []QuotationLineInput{{Description: "Instalasi", Quantity: "1", UnitPrice: "100000"}}}
}

func TestQuotationCreateRejectedWhenMonthlyQuotaFull(t *testing.T) {
	repo := &fakeQuotationRepo{stored: map[string]domain.Quotation{}}
	room := &fakeRoom{limit: 0}
	svc := NewQuotationService(repo, fakeCounter{}, WithQuotationQuotaEnforcer(room))

	_, err := svc.Create(context.Background(), testScope(t), quotationInput())
	if !isQuotaExceeded(err) {
		t.Fatalf("Create() error = %v, want QUOTA_EXCEEDED", err)
	}
	if len(repo.created) != 0 {
		t.Error("repository Create called despite full quota")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxQuotationsPerMonth {
		t.Errorf("enforcer keys = %v", room.keys)
	}
}

func TestQuotationCreateAllowedWhenRoom(t *testing.T) {
	repo := &fakeQuotationRepo{stored: map[string]domain.Quotation{}}
	svc := NewQuotationService(repo, fakeCounter{}, WithQuotationQuotaEnforcer(&fakeRoom{limit: 1}))
	if _, err := svc.Create(context.Background(), testScope(t), quotationInput()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(repo.created) != 1 {
		t.Error("repository Create not called")
	}
}

func TestQuotationWithoutEnforcerCreatesFreely(t *testing.T) {
	repo := &fakeQuotationRepo{stored: map[string]domain.Quotation{}}
	svc := NewQuotationService(repo, fakeCounter{})
	if _, err := svc.Create(context.Background(), testScope(t), quotationInput()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestQuotationReviseAndUpdateIgnoreQuota(t *testing.T) {
	repo := &fakeQuotationRepo{stored: map[string]domain.Quotation{
		"q1": {ID: "q1", QuotationNumber: "QUO-2026-0001", Status: domain.QuotationStatusSent,
			Items: []domain.QuotationItem{{Description: "A", Quantity: "1", UnitPrice: "10"}}},
	}}
	room := &fakeRoom{limit: 0}
	svc := NewQuotationService(repo, fakeCounter{}, WithQuotationQuotaEnforcer(room))

	if _, err := svc.Revise(context.Background(), testScope(t), "q1", "u1"); err != nil {
		t.Fatalf("Revise() error = %v, want success despite full quota", err)
	}
	if _, err := svc.Update(context.Background(), testScope(t), "q1", UpdateQuotationInput{}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted on Revise/Update: %v", room.keys)
	}
}

func newQuotaSOHarness(t *testing.T, room QuotaRoomRequirer) *soHarness {
	t.Helper()
	h := newSOHarness(t)
	h.svc = NewSalesOrderService(h.repo, h.billing, SalesOrderDeps{
		Counters: &soCounterFake{}, Contacts: soContacts{}, Deals: soDeals{}, Members: soMembers{}, Activities: h.activities,
		Notifier: h.notifier, FrontendURL: "https://app.test",
	}, h.won, func() time.Time { return soNow }, WithSalesOrderQuotaEnforcer(room))
	return h
}

func TestSalesOrderCreateRejectedWhenMonthlyQuotaFull(t *testing.T) {
	room := &fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}}
	h := newQuotaSOHarness(t, room)

	err := h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales")
	if !isQuotaExceeded(err) {
		t.Fatalf("QuotationApproved() error = %v, want QUOTA_EXCEEDED", err)
	}
	if len(h.repo.byID) != 0 {
		t.Error("sales order created despite full quota")
	}
	if len(h.notifier.events) != 0 || len(h.activities.types) != 0 {
		t.Error("side effects emitted for a rejected sales order")
	}
	if len(room.keys) != 1 || room.keys[0] != domain.FeatureCRMMaxSalesOrdersPerMonth {
		t.Errorf("enforcer keys = %v", room.keys)
	}
}

func TestSalesOrderHookIsIdempotentEvenWhenQuotaFull(t *testing.T) {
	room := &fakeRoom{limit: 100}
	h := newQuotaSOHarness(t, room)
	h.draft(t) // creates the SO while there is room

	room.fullKeys = map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}
	room.keys = nil
	if err := h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales"); err != nil {
		t.Fatalf("repeat QuotationApproved() error = %v, want nil (SO already exists)", err)
	}
	if err := h.svc.(SalesOrderRoomChecker).RequireRoomForQuotation(soCtx, soScope, "q1"); err != nil {
		t.Fatalf("RequireRoomForQuotation() for existing SO error = %v", err)
	}
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted although the SO already exists: %v", room.keys)
	}
}

func TestSalesOrderConfirmAndBillingIgnoreQuota(t *testing.T) {
	room := &fakeRoom{limit: 100}
	h := newQuotaSOHarness(t, room)
	so := h.draft(t)
	h.fill(t, so.ID, "2026-10-01")

	room.fullKeys = map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}
	room.keys = nil
	if _, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1"); err != nil {
		t.Fatalf("Confirm() error = %v, want success despite full quota", err)
	}
	if len(room.keys) != 0 {
		t.Errorf("enforcer consulted on Confirm: %v", room.keys)
	}
}

// Approving a quotation creates the sales order. When the SO quota is full the
// approval must fail BEFORE the quotation changes state (no half state).
func TestApproveQuotationRejectedBeforeStateChangeWhenSalesOrderQuotaFull(t *testing.T) {
	room := &fakeRoom{limit: 100, fullKeys: map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}}
	so := newQuotaSOHarness(t, room)
	repo := &quotaQuotationRepo{fakeQuotationRepo: fakeQuotationRepo{stored: map[string]domain.Quotation{
		"q1": {ID: "q1", Status: domain.QuotationStatusSent, Channel: domain.QuotationChannelSelfServe},
		"q2": {ID: "q2", Status: domain.QuotationStatusDraft, Channel: domain.QuotationChannelSelfServe},
	}}}
	svc := NewQuotationService(repo, fakeCounter{},
		WithQuotationApprovedHook(so.svc),
		WithQuotationOrderRoom(so.svc.(SalesOrderRoomChecker)))

	if _, err := svc.Approve(context.Background(), soScope, "q1", "u1"); !isQuotaExceeded(err) {
		t.Fatalf("Approve() error = %v, want QUOTA_EXCEEDED", err)
	}
	if _, err := svc.AcceptOnline(context.Background(), soScope, "q2", "u1"); !isQuotaExceeded(err) {
		t.Fatalf("AcceptOnline() error = %v, want QUOTA_EXCEEDED", err)
	}
	if repo.approved != 0 || repo.accepted != 0 {
		t.Fatalf("quotation state changed: approved=%d accepted=%d", repo.approved, repo.accepted)
	}
	if len(so.repo.byID) != 0 {
		t.Error("sales order created")
	}

	// With room, the very same flow goes through and creates the order.
	room.fullKeys = nil // limit 100, used 0
	if _, err := svc.Approve(context.Background(), soScope, "q1", "u1"); err != nil {
		t.Fatalf("Approve() with room error = %v", err)
	}
	if repo.approved != 1 {
		t.Errorf("approved = %d, want 1", repo.approved)
	}
}

// A customer's public approval is never refused because of the tenant's sales
// order quota: the quotation is approved, the SO is still created (and counted
// by the meter) and a warning is logged for the tenant.
func TestPublicApproveCreatesSalesOrderEvenWhenSalesOrderQuotaFull(t *testing.T) {
	h := newPublicHarness(t)
	room := &fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}}
	so := newQuotaSOHarness(t, room)
	svc := NewPublicQuotationService(h.links, publicScopes{}, h.stub, &publicResponses{stub: h.stub}, fakeIssuer{}, h.activities,
		h.notifier, so.svc, func() time.Time { return h.now })

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	view, err := svc.Approve(context.Background(), "tok1", PublicResponseInput{ResponderName: "Budi Santoso"})
	if err != nil {
		t.Fatalf("Approve() error = %v, want nil (customer approval is never blocked)", err)
	}
	if view.Status != domain.QuotationStatusApproved || h.stub.q.Status != domain.QuotationStatusApproved {
		t.Fatalf("quotation status = %s / %s, want approved", view.Status, h.stub.q.Status)
	}
	if len(so.repo.byID) != 1 {
		t.Fatalf("sales orders = %d, want 1 (created despite full quota)", len(so.repo.byID))
	}
	out := logs.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "quotation_id=q1") ||
		!strings.Contains(out, "organization_id=11111111-1111-1111-1111-111111111111") {
		t.Fatalf("missing quota warning, logs:\n%s", out)
	}
}

// Outside a customer approval the defense-in-depth check still refuses.
func TestSalesOrderHookStillEnforcedOutsideCustomerApproval(t *testing.T) {
	room := &fakeRoom{fullKeys: map[string]bool{domain.FeatureCRMMaxSalesOrdersPerMonth: true}}
	h := newQuotaSOHarness(t, room)
	if err := h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales"); !isQuotaExceeded(err) {
		t.Fatalf("QuotationApproved() error = %v, want QUOTA_EXCEEDED", err)
	}
}

// A full invoice quota must not look like a transient failure: the order stays
// retryable (billing_status=failed) but carries a message that says why.
func TestConfirmBillingQuotaFullStoresQuotaMessageAndStaysRetryable(t *testing.T) {
	h := newSOHarness(t)
	so := h.draft(t)
	h.fill(t, so.ID, "2026-10-05")
	h.billing.err = fmt.Errorf("create invoice: %w", organization.QuotaExceededError())

	got, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1")
	if err != nil || got.Status != domain.SalesOrderConfirmed || got.BillingStatus != domain.BillingFailed {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got.BillingError != billingQuotaMessage || got.BillingError == billingFailedMessage {
		t.Fatalf("BillingError = %q, want quota message", got.BillingError)
	}

	h.billing.err = nil // room again (new month / upgrade)
	got, err = h.svc.RetryBilling(soCtx, soScope, so.ID, "u1")
	if err != nil || got.BillingStatus != domain.BillingDone {
		t.Fatalf("retry got=%+v err=%v", got, err)
	}
}
