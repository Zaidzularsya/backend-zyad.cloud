package service

import (
	"context"
	"fmt"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

// ReceivableListener menghubungkan peristiwa receivable ke evaluator Won dan ke akses workspace (TenantAccess). Dipanggil setelah commit
// pembayaran/kontrak: panic dan galat hanya di-log. Won tetap dipulihkan saat Deal Detail membuka
// won-checklist (yang menjalankan Evaluate ulang).
type ReceivableListener struct {
	orders     repository.SalesOrderRepository
	won        DealWonEvaluator
	activities repository.ActivityRepository
	access     ContractSyncer // nil = tidak menyinkronkan akses workspace
}

var _ receivableservice.Listener = (*ReceivableListener)(nil)

func NewReceivableListener(orders repository.SalesOrderRepository, won DealWonEvaluator, activities repository.ActivityRepository) *ReceivableListener {
	return &ReceivableListener{orders: orders, won: won, activities: activities}
}

// WithTenantAccess mengaktifkan sinkronisasi entitlement workspace dari peristiwa contract/invoice.
func (l *ReceivableListener) WithTenantAccess(access ContractSyncer) *ReceivableListener {
	l.access = access
	return l
}

func (l *ReceivableListener) syncAccess(ctx context.Context, scope coretenant.Scope, contractID string, reason SyncReason) {
	if l.access == nil || contractID == "" {
		return
	}
	if err := l.access.SyncContract(ctx, scope, contractID, reason); err != nil {
		slog.Warn("listener: tenant access sync failed", "contract_id", contractID, "reason", string(reason), "error", err)
	}
}

func (l *ReceivableListener) guard(what string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("crm receivable listener panicked", "event", what, "panic", rec)
		}
	}()
	fn()
}

func (l *ReceivableListener) InvoicePaid(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.guard("invoice paid", func() {
		l.syncAccess(ctx, scope, inv.ContractID, SyncInvoicePaid)
		// Invoice berkala dari kontrak tidak memengaruhi syarat Won (kontrak ada = terpenuhi).
		if inv.SourceType != receivabledomain.SourceSalesOrder {
			return
		}
		so, err := l.orders.FindByID(ctx, scope, inv.SourceID)
		if err != nil || so.DealID == nil {
			return
		}
		if l.activities != nil {
			if _, err := l.activities.Create(ctx, scope, repository.CreateActivityParams{
				RelatedEntityType: domain.ActivityEntityDeal, RelatedEntityID: *so.DealID, Type: domain.ActivityTypeOrder,
				Subject: fmt.Sprintf("Invoice %s lunas", inv.Number), Status: domain.ActivityStatusCompleted,
			}); err != nil {
				slog.Warn("listener: paid activity failed", "error", err)
			}
		}
		if _, err := l.won.Evaluate(ctx, scope, *so.DealID); err != nil {
			slog.Warn("listener: won evaluation failed", "deal_id", *so.DealID, "error", err)
		}
	})
}

func (l *ReceivableListener) ContractEnded(ctx context.Context, scope coretenant.Scope, c receivableservice.ContractRef) {
	l.guard("contract ended", func() { l.syncAccess(ctx, scope, c.ID, SyncContractEnded) })
}

func (l *ReceivableListener) ContractCreated(ctx context.Context, scope coretenant.Scope, c receivableservice.ContractRef) {
	l.guard("contract created", func() {
		l.syncAccess(ctx, scope, c.ID, SyncContractCreated)
		if c.SourceType != receivabledomain.SourceSalesOrder {
			return
		}
		so, err := l.orders.FindByID(ctx, scope, c.SourceID)
		if err != nil || so.DealID == nil {
			return
		}
		if _, err := l.won.Evaluate(ctx, scope, *so.DealID); err != nil {
			slog.Warn("listener: won evaluation failed", "deal_id", *so.DealID, "error", err)
		}
	})
}
