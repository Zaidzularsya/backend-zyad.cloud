package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
)

// SyncReason hanya dipakai untuk log: apa yang memicu sinkronisasi akses workspace.
type SyncReason string

const (
	SyncInvoicePaid     SyncReason = "invoice_paid"
	SyncContractCreated SyncReason = "contract_created"
	SyncContractEnded   SyncReason = "contract_ended"
	SyncWorkspaceLinked SyncReason = "workspace_linked"
)

// ContractAccessItem adalah item contract berulang beserta snapshot fitur produknya (urut position).
type ContractAccessItem struct {
	Prepaid  bool
	Features []domain.FeatureSnapshot
}

type ContractAccessInfo struct {
	ID, Number, Status, SourceType, SourceID string
	Items                                    []ContractAccessItem
}

// ContractAccessReader: pembacaan sempit ke modul receivable (diadaptasi di internal/app).
type ContractAccessReader interface {
	ContractAccess(ctx context.Context, scope coretenant.Scope, contractID string) (ContractAccessInfo, error)
	// ContractHasPaidInvoice: ada invoice contract itu (awal maupun periode) yang sudah lunas.
	ContractHasPaidInvoice(ctx context.Context, scope coretenant.Scope, contractID string) (bool, error)
}

// TenantEntitlementWriter menulis entitlement di workspace pelanggan (diadaptasi di internal/app di atas
// organization.ContractEntitlementService). Semua metode idempoten.
type TenantEntitlementWriter interface {
	GrantContract(ctx context.Context, tenantOrgID, contractID, contractNumber string, features []domain.FeatureSnapshot, actorUserID string) error
	// RevokeContract mencabut fitur contract; bila workspace tidak punya contract lain, paket default dikembalikan.
	RevokeContract(ctx context.Context, tenantOrgID, contractID, contractNumber string, actorUserID string) error
}

// ContractSyncer dipanggil listener setiap kali status bayar/contract berubah.
type ContractSyncer interface {
	SyncContract(ctx context.Context, scope coretenant.Scope, contractID string, reason SyncReason) error
}

// TenantAccess menerjemahkan keadaan contract menjadi akses workspace: company deal → workspace tertaut →
// entitlement. Company tanpa tautan workspace tidak diberi apa pun.
type TenantAccess struct {
	orders    repository.SalesOrderRepository
	companies repository.CompanyRepository
	contracts ContractAccessReader
	invoices  InvoiceStatusReader
	writer    TenantEntitlementWriter
}

var _ ContractSyncer = (*TenantAccess)(nil)

func NewTenantAccess(
	orders repository.SalesOrderRepository,
	companies repository.CompanyRepository,
	contracts ContractAccessReader,
	invoices InvoiceStatusReader,
	writer TenantEntitlementWriter,
) *TenantAccess {
	return &TenantAccess{orders: orders, companies: companies, contracts: contracts, invoices: invoices, writer: writer}
}

// SyncContract membuat akses workspace sesuai keadaan contract saat ini:
//   - contract ended/cancelled → cabut;
//   - item pascabayar dihitung sejak contract ada, item prabayar hanya setelah ada invoice contract yang lunas;
//   - tidak ada item yang dihitung → tidak ada aksi.
func (a *TenantAccess) SyncContract(ctx context.Context, scope coretenant.Scope, contractID string, reason SyncReason) error {
	info, err := a.contracts.ContractAccess(ctx, scope, contractID)
	if err != nil {
		return fmt.Errorf("read contract: %w", err)
	}
	if receivabledomain.SourceType(info.SourceType) != receivabledomain.SourceSalesOrder {
		return nil
	}
	so, err := a.orders.FindByID(ctx, scope, info.SourceID)
	if err != nil {
		return fmt.Errorf("read sales order: %w", err)
	}
	if so.CompanyID == nil {
		return nil
	}
	company, err := a.companies.FindByID(ctx, scope, *so.CompanyID)
	if err != nil {
		return fmt.Errorf("read company: %w", err)
	}
	if company.TenantOrganizationID == nil || *company.TenantOrganizationID == "" {
		return nil
	}
	tenantOrgID := *company.TenantOrganizationID

	slog.DebugContext(ctx, "tenant access sync", "contract_id", contractID, "reason", string(reason), "tenant_organization_id", tenantOrgID)
	switch receivabledomain.ContractStatus(info.Status) {
	case receivabledomain.ContractEnded, receivabledomain.ContractCancelled:
		return a.writer.RevokeContract(ctx, tenantOrgID, info.ID, info.Number, "")
	}

	paid, err := a.prepaidSettled(ctx, scope, info, so)
	if err != nil {
		return err
	}
	counted := make([]ContractAccessItem, 0, len(info.Items))
	for _, item := range info.Items {
		if !item.Prepaid || paid {
			counted = append(counted, item)
		}
	}
	features := MergeFeatures(counted)
	if len(features) == 0 {
		return nil
	}
	return a.writer.GrantContract(ctx, tenantOrgID, info.ID, info.Number, features, "")
}

// prepaidSettled: invoice contract lunas, dilihat dari invoice periode/awal berkontrak atau invoice awal SO.
func (a *TenantAccess) prepaidSettled(ctx context.Context, scope coretenant.Scope, info ContractAccessInfo, so domain.SalesOrder) (bool, error) {
	hasPrepaid := false
	for _, item := range info.Items {
		hasPrepaid = hasPrepaid || item.Prepaid
	}
	if !hasPrepaid {
		return false, nil
	}
	paid, err := a.contracts.ContractHasPaidInvoice(ctx, scope, info.ID)
	if err != nil {
		return false, fmt.Errorf("read contract invoices: %w", err)
	}
	if paid || so.InitialInvoiceID == "" || a.invoices == nil {
		return paid, nil
	}
	status, _, err := a.invoices.InvoiceStatus(ctx, scope, so.InitialInvoiceID)
	if err != nil {
		return false, fmt.Errorf("read initial invoice: %w", err)
	}
	return receivabledomain.InvoiceStatus(status) == receivabledomain.InvoicePaid, nil
}

// MergeFeatures menggabungkan fitur beberapa item contract menjadi satu daftar per fitur (urutan kemunculan
// pertama dipertahankan). Tipe nilai dikenali dari JSON-nya: boolean → true menang; angka → terbesar;
// string desimal → terbesar; string lain → nilai item dengan position terkecil (item pertama).
func MergeFeatures(items []ContractAccessItem) []domain.FeatureSnapshot {
	var order []string
	merged := map[string]domain.FeatureSnapshot{}
	for _, item := range items {
		for _, f := range item.Features {
			current, ok := merged[f.FeatureKey]
			if !ok {
				order = append(order, f.FeatureKey)
				merged[f.FeatureKey] = f
				continue
			}
			if preferNew(current.Value, f.Value) {
				merged[f.FeatureKey] = f
			}
		}
	}
	out := make([]domain.FeatureSnapshot, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	return out
}

// preferNew: apakah nilai kedua harus menggantikan nilai pertama.
func preferNew(current, next json.RawMessage) bool {
	current, next = bytes.TrimSpace(current), bytes.TrimSpace(next)
	var cb, nb bool
	if json.Unmarshal(current, &cb) == nil && json.Unmarshal(next, &nb) == nil {
		return nb && !cb
	}
	if cr, ok := ratOf(current); ok {
		if nr, ok := ratOf(next); ok {
			return nr.Cmp(cr) > 0
		}
	}
	return false
}

func ratOf(raw json.RawMessage) (*big.Rat, bool) {
	text := string(raw)
	var s string
	if json.Unmarshal(raw, &s) == nil {
		text = s
	}
	return new(big.Rat).SetString(text)
}

var _ WorkspaceLinkHook = (*TenantAccess)(nil)

// OnWorkspaceLinkChanged memindahkan akses saat company ditautkan, diganti, atau dilepas dari workspace:
// contract company dicabut dari workspace lama (yang lalu kembali ke paket default bila tidak punya contract
// lain) dan diberikan ke workspace baru sesuai status bayarnya.
func (a *TenantAccess) OnWorkspaceLinkChanged(ctx context.Context, scope coretenant.Scope, companyID string, oldTenantOrgID, newTenantOrgID *string) error {
	const pageSize = 200
	var errs []error
	for offset := 0; ; offset += pageSize {
		orders, total, err := a.orders.List(ctx, scope, repository.SalesOrderListFilter{CompanyID: companyID, Limit: pageSize, Offset: offset})
		if err != nil {
			return fmt.Errorf("list company sales orders: %w", err)
		}
		for _, so := range orders {
			if so.ContractID == "" {
				continue
			}
			if oldTenantOrgID != nil {
				info, err := a.contracts.ContractAccess(ctx, scope, so.ContractID)
				if err != nil {
					errs = append(errs, fmt.Errorf("read contract %s: %w", so.ContractID, err))
				} else if err := a.writer.RevokeContract(ctx, *oldTenantOrgID, info.ID, info.Number, ""); err != nil {
					errs = append(errs, fmt.Errorf("revoke contract %s: %w", so.ContractID, err))
				}
			}
			if newTenantOrgID != nil {
				if err := a.SyncContract(ctx, scope, so.ContractID, SyncWorkspaceLinked); err != nil {
					errs = append(errs, fmt.Errorf("sync contract %s: %w", so.ContractID, err))
				}
			}
		}
		if int64(offset+len(orders)) >= total || len(orders) == 0 {
			break
		}
	}
	return errors.Join(errs...)
}
