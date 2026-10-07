package app

import (
	"context"
	"encoding/json"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	productrepo "zyad.cloud/internal/modules/product/repository"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

// newCRMTenantAccess merakit sinkronisasi contract → entitlement workspace. Dipakai API (DOKU webhook,
// End contract, tautan company) dan worker (billing run mengakhiri contract).
func newCRMTenantAccess(
	db *database.Pool,
	invoices receivableservice.InvoiceService,
	contracts receivableservice.ContractService,
	defaults defaultAccessGranter,
) *crmservice.TenantAccess {
	reader := contractAccessReader{invoices: invoices, contracts: contracts}
	writer := tenantEntitlementWriter{
		svc:      organizationservice.NewContractEntitlementService(organizationrepo.NewEntitlementRepository(db)),
		registry: catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)},
		defaults: defaults,
	}
	return crmservice.NewTenantAccess(
		crmrepo.NewSalesOrderRepository(db), crmrepo.NewCompanyRepository(db),
		reader, crmReceivableReader{invoices: invoices, contracts: contracts}, writer,
	)
}

// defaultAccessGranter mengembalikan paket gratis ke workspace (nil = tidak mengembalikan).
type defaultAccessGranter interface {
	ProvisionDefaultAccess(ctx context.Context, organizationID string) error
}

// contractAccessReader adalah adapter tipis CRM → receivable untuk SyncContract.
type contractAccessReader struct {
	invoices  receivableservice.InvoiceService
	contracts receivableservice.ContractService
}

func (r contractAccessReader) ContractAccess(ctx context.Context, scope coretenant.Scope, id string) (crmservice.ContractAccessInfo, error) {
	c, err := r.contracts.Get(ctx, scope, id)
	if err != nil {
		return crmservice.ContractAccessInfo{}, err
	}
	info := crmservice.ContractAccessInfo{
		ID: c.ID, Number: c.ContractNumber, Status: string(c.Status), SourceType: string(c.SourceType), SourceID: c.SourceID,
	}
	for _, it := range c.Items {
		item := crmservice.ContractAccessItem{Prepaid: it.PaymentTiming == pricing.Prepaid}
		if len(it.Features) > 0 {
			if err := json.Unmarshal(it.Features, &item.Features); err != nil {
				slog.WarnContext(ctx, "contract item features unreadable", "contract_id", id, "item_id", it.ID, "error", err)
			}
		}
		info.Items = append(info.Items, item)
	}
	return info, nil
}

func (r contractAccessReader) ContractHasPaidInvoice(ctx context.Context, scope coretenant.Scope, id string) (bool, error) {
	_, total, err := r.invoices.List(ctx, scope, receivablerepo.InvoiceListFilter{ContractID: id, Status: "paid", Limit: 1})
	return total > 0, err
}

// tenantEntitlementWriter memetakan snapshot fitur (kunci + nilai JSON) ke FeatureGrant memakai tipe nilai
// dari registry fitur, lalu menulisnya lewat ContractEntitlementService.
type tenantEntitlementWriter struct {
	svc      *organizationservice.ContractEntitlementService
	registry catalogFeatureRegistry
	defaults defaultAccessGranter
}

func (w tenantEntitlementWriter) GrantContract(
	ctx context.Context, tenantOrgID, contractID, contractNumber string, features []crmdomain.FeatureSnapshot, actorUserID string,
) error {
	grants, err := w.grants(ctx, features)
	if err != nil {
		return err
	}
	return w.svc.GrantContract(ctx, tenantOrgID, contractID, contractNumber, grants, actorUserID)
}

func (w tenantEntitlementWriter) RevokeContract(
	ctx context.Context, tenantOrgID, contractID, contractNumber, actorUserID string,
) error {
	remaining, err := w.svc.RevokeContract(ctx, tenantOrgID, contractID, contractNumber, actorUserID)
	if err != nil || remaining || w.defaults == nil {
		return err
	}
	// Gagal mengembalikan paket gratis tidak menggagalkan pencabutan: dicatat, bisa diulang lewat sinkronisasi berikutnya.
	if err := w.defaults.ProvisionDefaultAccess(ctx, tenantOrgID); err != nil {
		slog.WarnContext(ctx, "restore default access after contract end failed", "organization_id", tenantOrgID, "error", err)
	}
	return nil
}

func (w tenantEntitlementWriter) grants(ctx context.Context, features []crmdomain.FeatureSnapshot) ([]organizationservice.FeatureGrant, error) {
	keys := make([]string, len(features))
	for i, f := range features {
		keys[i] = f.FeatureKey
	}
	defs, err := w.registry.FindByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	return featureGrants(features, defs), nil
}

// featureGrants melewati fitur yang sudah tidak ada di registry (dicatat), supaya satu fitur usang tidak
// menggagalkan pemberian fitur lain.
func featureGrants(features []crmdomain.FeatureSnapshot, defs map[string]catalogdomain.FeatureDef) []organizationservice.FeatureGrant {
	grants := make([]organizationservice.FeatureGrant, 0, len(features))
	for _, f := range features {
		def, ok := defs[f.FeatureKey]
		if !ok {
			slog.Warn("feature missing from registry; skipped", "feature_key", f.FeatureKey)
			continue
		}
		grants = append(grants, organizationservice.FeatureGrant{Key: f.FeatureKey, ValueType: def.ValueType, Value: f.Value})
	}
	return grants
}
