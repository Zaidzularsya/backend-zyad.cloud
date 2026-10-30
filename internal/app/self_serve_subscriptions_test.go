package app

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

type subOrdersFake struct {
	crmrepo.SalesOrderRepository
	orders map[string]crmdomain.SalesOrder
}

func (f subOrdersFake) List(context.Context, coretenant.Scope, crmrepo.SalesOrderListFilter) ([]crmdomain.SalesOrder, int64, error) {
	out := make([]crmdomain.SalesOrder, 0, len(f.orders))
	for _, so := range f.orders {
		so.Items = nil // List tidak memuat item
		out = append(out, so)
	}
	return out, int64(len(out)), nil
}

func (f subOrdersFake) FindByID(_ context.Context, _ coretenant.Scope, id string) (crmdomain.SalesOrder, error) {
	return f.orders[id], nil
}

type subContractsFake struct {
	receivableservice.ContractService
	status map[string]receivabledomain.ContractStatus
}

func (f subContractsFake) Get(_ context.Context, _ coretenant.Scope, id string) (receivabledomain.Contract, error) {
	return receivabledomain.Contract{ID: id, Status: f.status[id]}, nil
}

type subProductsFake struct {
	products map[string]catalogdomain.Product
}

func (f subProductsFake) FindByIDs(_ context.Context, _ coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error) {
	out := map[string]catalogdomain.Product{}
	for _, id := range ids {
		if p, ok := f.products[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func TestSelfServeSubscriptionsByProduct(t *testing.T) {
	s := selfServeSubscriptions{
		orders: subOrdersFake{orders: map[string]crmdomain.SalesOrder{
			"so-active": {ID: "so-active", Status: crmdomain.SalesOrderConfirmed, ContractID: "c-active",
				Items: []crmdomain.SalesOrderItem{{ProductID: "wifi-10"}}},
			"so-ended": {ID: "so-ended", Status: crmdomain.SalesOrderCompleted, ContractID: "c-ended",
				Items: []crmdomain.SalesOrderItem{{ProductID: "wifi-20"}}},
		}},
		contracts: crmReceivableReader{contracts: subContractsFake{status: map[string]receivabledomain.ContractStatus{
			"c-active": receivabledomain.ContractActive, "c-ended": receivabledomain.ContractEnded,
		}}},
		products: subProductsFake{products: map[string]catalogdomain.Product{"wifi-10": {ID: "wifi-10"}}}, // tanpa fitur
	}
	ctx := context.Background()
	for product, want := range map[string]bool{"wifi-10": true, "wifi-20": false, "wifi-50": false} {
		got, err := s.HasActiveContractForProduct(ctx, coretenant.Scope{}, "co-1", product)
		if err != nil || got != want {
			t.Fatalf("%s: got=%v err=%v want %v", product, got, err, want)
		}
	}
	// Produk tanpa fitur tidak pernah terdeteksi oleh pemeriksaan berfitur (alasan varian per produk).
	if got, err := s.HasActiveFeatureContract(ctx, coretenant.Scope{}, "co-1"); err != nil || got {
		t.Fatalf("feature check: got=%v err=%v", got, err)
	}
}
