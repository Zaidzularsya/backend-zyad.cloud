package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

func TestBillsListsSellerInvoicesWithoutDrafts(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i1", ContractID: "ctr-1"}
	r := subReader{
		bySource: []BillingInvoice{subInv("i1", "INV-1", "paid", "2026-10-12"), subInv("i0", "INV-0", "draft", "")},
		byCtr:    []BillingInvoice{subInv("i1", "INV-1", "paid", "2026-10-12"), subInv("i2", "INV-2", "issued", "2026-11-12")},
	}
	got, err := newSubAccess(true, so, r).Bills(context.Background(), coretenant.Scope{}, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	numbers := map[string]string{}
	for _, inv := range got {
		numbers[inv.Number] = inv.Status
		if inv.URL == "" {
			t.Fatalf("invoice %s harus punya link bayar/lihat", inv.Number)
		}
	}
	if len(got) != 2 || numbers["INV-1"] != "paid" || numbers["INV-2"] != "issued" {
		t.Fatalf("bills = %v (draft tidak boleh tampil, invoice ganda dilebur)", numbers)
	}
}

func TestBillsEmptyWhenWorkspaceNotLinkedToCompany(t *testing.T) {
	got, err := newSubAccess(false, domain.SalesOrder{}, subReader{}).Bills(context.Background(), coretenant.Scope{}, "ws-1")
	if err != nil || len(got) != 0 {
		t.Fatalf("tak tertaut = %v err=%v", got, err)
	}
}

func TestBillsOnlyForRequestedWorkspace(t *testing.T) {
	so := domain.SalesOrder{ID: "so-1", InitialInvoiceID: "i1"}
	r := subReader{bySource: []BillingInvoice{subInv("i1", "INV-1", "issued", "2026-10-12")}}
	// subCompanies hanya menautkan ws-1; workspace lain tidak boleh melihat tagihan itu.
	got, err := newSubAccess(true, so, r).Bills(context.Background(), coretenant.Scope{}, "ws-lain")
	if err != nil || len(got) != 0 {
		t.Fatalf("workspace lain = %v err=%v", got, err)
	}
}

func TestBillsRequiresSubscriptionReader(t *testing.T) {
	access := NewTenantAccess(subOrders{}, subCompanies{linked: true}, taContracts{}, nil, &taWriter{})
	if _, err := access.Bills(context.Background(), coretenant.Scope{}, "ws-1"); err == nil {
		t.Fatal("tanpa reader harus galat, bukan panic")
	}
}
