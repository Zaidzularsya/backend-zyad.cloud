package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

type taOrders struct {
	repository.SalesOrderRepository
	so domain.SalesOrder
}

func (o taOrders) FindByID(context.Context, coretenant.Scope, string) (domain.SalesOrder, error) {
	return o.so, nil
}

type taCompanies struct {
	repository.CompanyRepository
	company domain.Company
}

func (c taCompanies) FindByID(context.Context, coretenant.Scope, string) (domain.Company, error) {
	return c.company, nil
}

type taContracts struct {
	info ContractAccessInfo
	paid bool
}

func (c taContracts) ContractAccess(context.Context, coretenant.Scope, string) (ContractAccessInfo, error) {
	return c.info, nil
}
func (c taContracts) ContractHasPaidInvoice(context.Context, coretenant.Scope, string) (bool, error) {
	return c.paid, nil
}

type taInvoices struct{ status string }

func (i taInvoices) InvoiceStatus(context.Context, coretenant.Scope, string) (string, string, error) {
	return i.status, "INV-1", nil
}

type taWriter struct {
	grants  [][]domain.FeatureSnapshot
	orgs    []string
	revokes []string
}

func (w *taWriter) GrantContract(_ context.Context, org, _, _ string, f []domain.FeatureSnapshot, _ string) error {
	w.orgs = append(w.orgs, org)
	w.grants = append(w.grants, f)
	return nil
}
func (w *taWriter) RevokeContract(_ context.Context, org, contractID, _ string, _ string) error {
	w.revokes = append(w.revokes, org+"/"+contractID)
	return nil
}

func feature(key, value string) domain.FeatureSnapshot {
	return domain.FeatureSnapshot{FeatureKey: key, Value: json.RawMessage(value)}
}

type taFixture struct {
	access   *TenantAccess
	writer   *taWriter
	contract *taContracts
}

func newTenantAccessFixture(items []ContractAccessItem, linked bool) taFixture {
	org := "ws-1"
	company := domain.Company{}
	if linked {
		company.TenantOrganizationID = &org
	}
	companyID := "co-1"
	contracts := &taContracts{info: ContractAccessInfo{
		ID: "ctr-1", Number: "CTR-1", Status: "active", SourceType: string(receivabledomain.SourceSalesOrder), SourceID: "so-1", Items: items,
	}}
	writer := &taWriter{}
	return taFixture{
		access: NewTenantAccess(taOrders{so: domain.SalesOrder{ID: "so-1", CompanyID: &companyID, InitialInvoiceID: "inv-1"}},
			taCompanies{company: company}, contracts, taInvoices{status: "issued"}, writer),
		writer: writer, contract: contracts,
	}
}

func TestSyncContractPrepaidWaitsForPayment(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: true, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, true)
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncInvoicePaid); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants) != 0 {
		t.Fatalf("unpaid prepaid contract was granted: %+v", f.writer.grants)
	}
	f.contract.paid = true
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncInvoicePaid); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants) != 1 || f.writer.grants[0][0].FeatureKey != "user.max" || f.writer.orgs[0] != "ws-1" {
		t.Fatalf("grants = %+v orgs=%v", f.writer.grants, f.writer.orgs)
	}
}

func TestSyncContractPrepaidPaidViaInitialInvoice(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: true, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, true)
	f.access.invoices = taInvoices{status: "paid"}
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncWorkspaceLinked); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants) != 1 {
		t.Fatalf("grants = %+v, want 1", f.writer.grants)
	}
}

func TestSyncContractPostpaidGrantedOnCreate(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: false, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, true)
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncContractCreated); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants) != 1 {
		t.Fatalf("grants = %+v, want 1", f.writer.grants)
	}
}

func TestSyncContractSkipsUnlinkedCompany(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: false, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, false)
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncContractCreated); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants)+len(f.writer.revokes) != 0 {
		t.Fatalf("writer called for unlinked company: %+v", f.writer)
	}
}

func TestSyncContractRevokesEndedContract(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: false, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, true)
	f.contract.info.Status = "ended"
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncContractEnded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.writer.revokes, []string{"ws-1/ctr-1"}) || len(f.writer.grants) != 0 {
		t.Fatalf("revokes=%v grants=%v", f.writer.revokes, f.writer.grants)
	}
}

func TestSyncContractIgnoresNonSalesOrderContract(t *testing.T) {
	f := newTenantAccessFixture(nil, true)
	f.contract.info.SourceType = "manual"
	if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncContractCreated); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.grants)+len(f.writer.revokes) != 0 {
		t.Fatalf("writer called: %+v", f.writer)
	}
}

func TestSyncContractRepeatedPaidEventGrantsSameArguments(t *testing.T) {
	f := newTenantAccessFixture([]ContractAccessItem{{Prepaid: true, Features: []domain.FeatureSnapshot{feature("user.max", "5")}}}, true)
	f.contract.paid = true
	for i := 0; i < 2; i++ {
		if err := f.access.SyncContract(context.Background(), coretenant.Scope{}, "ctr-1", SyncInvoicePaid); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.writer.grants) != 2 || !reflect.DeepEqual(f.writer.grants[0], f.writer.grants[1]) {
		t.Fatalf("grants = %+v, want two identical calls (writer is idempotent)", f.writer.grants)
	}
}

func TestMergeFeatures(t *testing.T) {
	items := []ContractAccessItem{
		{Features: []domain.FeatureSnapshot{feature("user.max", "3"), feature("crm.enabled", "false"), feature("plan.name", `"Basic"`), feature("storage.gb", `"1.5"`)}},
		{Features: []domain.FeatureSnapshot{feature("user.max", "5"), feature("crm.enabled", "true"), feature("plan.name", `"Pro"`), feature("storage.gb", `"20"`)}},
	}
	got := MergeFeatures(items)
	values := map[string]string{}
	for _, f := range got {
		values[f.FeatureKey] = string(f.Value)
	}
	want := map[string]string{"user.max": "5", "crm.enabled": "true", "plan.name": `"Basic"`, "storage.gb": `"20"`}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("merged = %v, want %v", values, want)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
}

type taSyncer struct {
	calls []string
	err   error
}

func (s *taSyncer) SyncContract(_ context.Context, _ coretenant.Scope, contractID string, reason SyncReason) error {
	s.calls = append(s.calls, contractID+":"+string(reason))
	return s.err
}

func TestReceivableListenerSyncsAccess(t *testing.T) {
	syncer := &taSyncer{err: errors.New("db down")}
	l := NewReceivableListener(taOrders{so: domain.SalesOrder{}}, nil, nil).WithTenantAccess(syncer)
	scope := coretenant.Scope{}
	// InvoicePaid tanpa ContractID (invoice one-time): tidak ada sync dan tidak panik walau won evaluator nil.
	l.InvoicePaid(context.Background(), scope, receivableservice.InvoiceRef{ID: "i1", SourceType: receivabledomain.SourceManual})
	l.InvoicePaid(context.Background(), scope, receivableservice.InvoiceRef{ID: "i2", ContractID: "ctr-1", SourceType: receivabledomain.SourceManual})
	l.ContractCreated(context.Background(), scope, receivableservice.ContractRef{ID: "ctr-2", SourceType: receivabledomain.SourceManual})
	l.ContractEnded(context.Background(), scope, receivableservice.ContractRef{ID: "ctr-3", SourceType: receivabledomain.SourceManual})
	want := []string{"ctr-1:invoice_paid", "ctr-2:contract_created", "ctr-3:contract_ended"}
	if !reflect.DeepEqual(syncer.calls, want) {
		t.Fatalf("calls = %v, want %v", syncer.calls, want)
	}
}
