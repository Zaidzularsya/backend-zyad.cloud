package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type subCompanies struct {
	repository.CompanyRepository
	linked bool
}

func (c subCompanies) FindByTenantOrganization(_ context.Context, _ coretenant.Scope, tenantOrgID string) (domain.Company, error) {
	if !c.linked || tenantOrgID != "ws-1" {
		return domain.Company{}, pgx.ErrNoRows
	}
	return domain.Company{ID: "co-1"}, nil
}

type subOrders struct {
	repository.SalesOrderRepository
	so domain.SalesOrder
}

func (o subOrders) List(context.Context, coretenant.Scope, repository.SalesOrderListFilter) ([]domain.SalesOrder, int64, error) {
	return []domain.SalesOrder{o.so}, 1, nil
}

type subReader struct {
	contract SubscriptionContract
	bySource []BillingInvoice
	byCtr    []BillingInvoice
}

func (r subReader) Contract(context.Context, coretenant.Scope, string) (SubscriptionContract, error) {
	return r.contract, nil
}
func (r subReader) InvoicesBySource(context.Context, coretenant.Scope, string, string) ([]BillingInvoice, error) {
	return r.bySource, nil
}
func (r subReader) InvoicesByContract(context.Context, coretenant.Scope, string) ([]BillingInvoice, error) {
	return r.byCtr, nil
}
func (r subReader) FreeFeatures(context.Context, coretenant.Scope) ([]domain.FeatureSnapshot, error) {
	return []domain.FeatureSnapshot{{FeatureKey: "user.max", Value: []byte("1"), Label: "1 pengguna"}}, nil
}
func (r subReader) Link(_ context.Context, _ coretenant.Scope, id string) (string, error) {
	return "https://app.test/i/" + id, nil
}

func subInv(id, number, status, due string) BillingInvoice {
	issue := day("2026-09-01")
	b := BillingInvoice{ID: id, Number: number, Status: status, GrandTotal: "333000.00", IssueDate: issue}
	if due != "" {
		b.DueDate = day(due)
	}
	return b
}

func newSubAccess(linked bool, so domain.SalesOrder, r subReader) *TenantAccess {
	return NewTenantAccess(subOrders{so: so}, subCompanies{linked: linked}, taContracts{}, nil, &taWriter{}).WithSubscription(r)
}

var subContract = SubscriptionContract{
	ID: "ctr-1", Number: "CTR-1", Status: "active", InvoiceLeadDays: 7,
	Items: []SubscriptionContractItem{{
		Name: "Starter", SKU: "STARTER", Frequency: "monthly", Prepaid: true, NextPeriodStart: *day("2026-11-05"),
		Features: []domain.FeatureSnapshot{{FeatureKey: "user.max", Value: []byte("5"), Label: "5 pengguna"}},
	}},
}

func TestSubscriptionFree(t *testing.T) {
	ctx, today := context.Background(), *day("2026-10-07")
	// workspace tidak tertaut ke company
	v, err := newSubAccess(false, domain.SalesOrder{}, subReader{}).Subscription(ctx, coretenant.Scope{}, "ws-1", today, 7)
	if err != nil || v.Status != "free" || len(v.Features) != 1 || v.Product != nil {
		t.Fatalf("unlinked = %+v err=%v", v, err)
	}
	// company tertaut tetapi belum ada SO/contract/invoice
	v, err = newSubAccess(true, domain.SalesOrder{ID: "so-1", Status: domain.SalesOrderCancelled}, subReader{}).Subscription(ctx, coretenant.Scope{}, "ws-1", today, 7)
	if err != nil || v.Status != "free" {
		t.Fatalf("no contract = %+v err=%v", v, err)
	}
}

func TestSubscriptionAwaitingPayment(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i1"}
	r := subReader{bySource: []BillingInvoice{subInv("i1", "INV-1", "issued", "2026-10-12")}}
	v, err := newSubAccess(true, so, r).Subscription(context.Background(), coretenant.Scope{}, "ws-1", *day("2026-10-07"), 7)
	if err != nil || v.Status != "awaiting_payment" || v.SuspendInDays != nil || len(v.Invoices) != 1 || v.Invoices[0].URL == "" {
		t.Fatalf("view = %+v err=%v", v, err)
	}
}

func TestSubscriptionActive(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i1", ContractID: "ctr-1"}
	r := subReader{contract: subContract,
		bySource: []BillingInvoice{subInv("i1", "INV-1", "paid", "2026-10-12")},
		byCtr:    []BillingInvoice{subInv("i1", "INV-1", "paid", "2026-10-12")}}
	v, err := newSubAccess(true, so, r).Subscription(context.Background(), coretenant.Scope{}, "ws-1", *day("2026-10-07"), 7)
	if err != nil || v.Status != "active" || v.ContractNumber != "CTR-1" || v.Product == nil || v.Product.SKU != "STARTER" {
		t.Fatalf("view = %+v err=%v", v, err)
	}
	if v.NextInvoiceDate == nil || !v.NextInvoiceDate.Equal(*day("2026-10-29")) {
		t.Fatalf("next invoice = %v, want 2026-10-29", v.NextInvoiceDate)
	}
	if len(v.Features) != 1 || v.Features[0].Label != "5 pengguna" || v.OverdueDays != 0 || v.SuspendInDays != nil {
		t.Fatalf("features/overdue = %+v", v)
	}
}

func TestSubscriptionOverdueSuspendCountdown(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i1", ContractID: "ctr-1"}
	cases := []struct {
		due         string
		overdueDays int
		suspendIn   int
	}{{"2026-10-04", 3, 5}, {"2026-09-29", 8, 0}, {"2026-09-01", 36, 0}}
	for _, c := range cases {
		inv := []BillingInvoice{subInv("i1", "INV-1", "paid", "2026-09-10"), subInv("i2", "INV-2", "overdue", c.due)}
		r := subReader{contract: subContract, bySource: inv[:1], byCtr: inv}
		v, err := newSubAccess(true, so, r).Subscription(context.Background(), coretenant.Scope{}, "ws-1", *day("2026-10-07"), 7)
		if err != nil || v.Status != "overdue" || v.OverdueDays != c.overdueDays || v.SuspendInDays == nil || *v.SuspendInDays != c.suspendIn {
			t.Fatalf("due %s: view = %+v err=%v", c.due, v, err)
		}
	}
}

func TestSubscriptionVoidInvoiceHasNoURLAndListIsCapped(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i0", ContractID: "ctr-1"}
	var all []BillingInvoice
	for i := 0; i < 15; i++ {
		inv := subInv("i"+string(rune('a'+i)), "INV-"+string(rune('A'+i)), "paid", "2026-09-10")
		d := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		inv.IssueDate = &d
		all = append(all, inv)
	}
	all[14].Status = "void"
	r := subReader{contract: subContract, bySource: all[:1], byCtr: all}
	v, err := newSubAccess(true, so, r).Subscription(context.Background(), coretenant.Scope{}, "ws-1", *day("2026-10-07"), 7)
	if err != nil || len(v.Invoices) != 12 {
		t.Fatalf("invoices = %d err=%v", len(v.Invoices), err)
	}
	if v.Invoices[0].Number != "INV-O" || v.Invoices[0].Status != "void" || v.Invoices[0].URL != "" {
		t.Fatalf("newest = %+v", v.Invoices[0])
	}
	if v.Invoices[1].URL == "" {
		t.Fatal("paid invoice must have a link")
	}
}
