package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/docpdf"
)

// BillingInvoice adalah ringkasan invoice receivable yang dibutuhkan untuk suspend/aktif kembali.
type BillingInvoice struct {
	ID, Number, Status, SourceType, SourceID, ContractID string
	DueDate                                              *time.Time
	AmountDue                                            string // sisa tagihan, desimal
}

// BillingInvoiceReader: pembacaan sempit ke modul receivable (diadaptasi di internal/app).
type BillingInvoiceReader interface {
	// OverdueDueBefore mengembalikan semua invoice org platform berstatus overdue dengan due_date < before.
	OverdueDueBefore(ctx context.Context, scope coretenant.Scope, before time.Time) ([]BillingInvoice, error)
	// Invoice mengembalikan pgx.ErrNoRows bila tidak ada.
	Invoice(ctx context.Context, scope coretenant.Scope, id string) (BillingInvoice, error)
	// Link membuat/mengambil link bayar publik invoice.
	Link(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (string, error)
}

// WithSuspend mengaktifkan suspend otomatis dan aktif kembali. actor menghasilkan user id bot untuk audit.
func (a *TenantAccess) WithSuspend(billing BillingInvoiceReader, status WorkspaceStatusPort, notifier AccessNotifier, actor func(context.Context) (string, error)) *TenantAccess {
	a.billing, a.status, a.notifier, a.actor = billing, status, notifier, actor
	return a
}

// SuspendResult: Checked = invoice overdue lewat grace yang diperiksa; Suspended = workspace yang ditangguhkan
// pada run ini; Skipped = invoice yang workspace-nya tidak ditangguhkan (tak tertaut, tak ada, bukan active, atau galat).
type SuspendResult struct{ Checked, Suspended, Skipped int }

// SuspendOverdue menangguhkan workspace aktif yang punya invoice overdue lewat grace. Kondisi lewat grace:
// due_date + graceDays < today. Workspace yang sudah tidak active (termasuk suspend manual admin) dilewati,
// jadi penanda billing tidak pernah ditulis di atas suspend manual dan run berulang tidak mengirim email ganda.
func (a *TenantAccess) SuspendOverdue(ctx context.Context, platform coretenant.Scope, today time.Time, graceDays int) (SuspendResult, error) {
	var res SuspendResult
	if a.billing == nil || a.status == nil {
		return res, errors.New("tenant access: suspend is not configured")
	}
	groups, order, unlinked, errs := a.overdueByWorkspace(ctx, platform, today, graceDays)
	for _, g := range groups {
		res.Checked += len(g)
	}
	res.Checked += unlinked.count
	res.Skipped += unlinked.count

	actor, err := a.actor(ctx)
	if err != nil {
		return res, fmt.Errorf("suspend actor: %w", err)
	}
	for _, wsID := range order {
		invs := groups[wsID]
		sort.SliceStable(invs, func(i, j int) bool { return invs[i].DueDate.Before(*invs[j].DueDate) })
		ref := invs[0]

		state, err := a.status.Get(ctx, wsID)
		if errors.Is(err, pgx.ErrNoRows) {
			slog.InfoContext(ctx, "suspend overdue: workspace not found", "organization_id", wsID)
			res.Skipped += len(invs)
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("workspace %s: %w", wsID, err))
			res.Skipped += len(invs)
			continue
		}
		if state.Status != "active" {
			res.Skipped += len(invs)
			continue
		}
		if err := a.status.SuspendForBilling(ctx, wsID, ref.Number, actor); err != nil {
			errs = append(errs, fmt.Errorf("suspend workspace %s: %w", wsID, err))
			res.Skipped += len(invs)
			continue
		}
		res.Suspended++

		url, err := a.billing.Link(ctx, platform, ref.ID, actor)
		if err != nil {
			slog.WarnContext(ctx, "suspend overdue: invoice link failed", "organization_id", wsID, "invoice_number", ref.Number, "error", err)
		}
		if a.notifier != nil {
			if err := a.notifier.WorkspaceSuspended(ctx, wsID, SuspendNotice{
				WorkspaceName: state.Name, InvoiceNumber: ref.Number, InvoiceURL: url,
				AmountDue: docpdf.FormatRupiah(ref.AmountDue), DueDate: *ref.DueDate, GraceDays: graceDays,
			}); err != nil {
				slog.WarnContext(ctx, "suspend overdue: notification failed", "organization_id", wsID, "invoice_number", ref.Number, "error", err)
			}
		}
	}
	return res, errors.Join(errs...)
}

// ReactivateIfSettled mengaktifkan kembali workspace yang ditangguhkan karena tagihan, setelah invoice ini lunas
// dan tidak ada lagi invoice overdue lewat grace untuk workspace yang sama. Suspend manual (tanpa penanda billing)
// tidak disentuh.
func (a *TenantAccess) ReactivateIfSettled(ctx context.Context, platform coretenant.Scope, invoiceID string, today time.Time, graceDays int) error {
	if a.billing == nil || a.status == nil {
		return nil
	}
	inv, err := a.billing.Invoice(ctx, platform, invoiceID)
	if err != nil {
		return fmt.Errorf("read invoice: %w", err)
	}
	wsID, ok, err := a.workspaceOfInvoice(ctx, platform, inv)
	if err != nil || !ok {
		return err
	}
	state, err := a.status.Get(ctx, wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}
	if state.Status != "suspended" || !state.BillingSuspended {
		return nil
	}
	groups, _, _, errs := a.overdueByWorkspace(ctx, platform, today, graceDays)
	if len(errs) > 0 {
		// Tidak yakin semua tunggakan terbaca: jangan membuka workspace; run berikutnya (invoice lain lunas) mencoba lagi.
		return fmt.Errorf("check remaining overdue invoices: %w", errors.Join(errs...))
	}
	if len(groups[wsID]) > 0 {
		return nil
	}
	actor, err := a.actor(ctx)
	if err != nil {
		return fmt.Errorf("reactivate actor: %w", err)
	}
	if err := a.status.ReactivateFromBilling(ctx, wsID, inv.Number, actor); err != nil {
		return fmt.Errorf("reactivate workspace %s: %w", wsID, err)
	}
	if a.notifier != nil {
		if err := a.notifier.WorkspaceReactivated(ctx, wsID, inv.Number); err != nil {
			slog.WarnContext(ctx, "reactivate: notification failed", "organization_id", wsID, "invoice_number", inv.Number, "error", err)
		}
	}
	return nil
}

type unlinkedCount struct{ count int }

// overdueByWorkspace mengelompokkan invoice overdue lewat grace per workspace tertaut. Invoice tanpa tautan
// workspace dihitung di unlinked; galat per invoice dikumpulkan tanpa menghentikan yang lain.
func (a *TenantAccess) overdueByWorkspace(ctx context.Context, platform coretenant.Scope, today time.Time, graceDays int) (map[string][]BillingInvoice, []string, unlinkedCount, []error) {
	groups := map[string][]BillingInvoice{}
	var order []string
	var unlinked unlinkedCount
	var errs []error
	cutoff := today.AddDate(0, 0, -graceDays)
	invoices, err := a.billing.OverdueDueBefore(ctx, platform, cutoff)
	if err != nil {
		return groups, order, unlinked, []error{fmt.Errorf("list overdue invoices: %w", err)}
	}
	for _, inv := range invoices {
		if inv.DueDate == nil {
			unlinked.count++
			continue
		}
		wsID, ok, err := a.workspaceOfInvoice(ctx, platform, inv)
		if err != nil {
			errs = append(errs, fmt.Errorf("invoice %s: %w", inv.Number, err))
			unlinked.count++
			continue
		}
		if !ok {
			unlinked.count++
			continue
		}
		if _, seen := groups[wsID]; !seen {
			order = append(order, wsID)
		}
		groups[wsID] = append(groups[wsID], inv)
	}
	return groups, order, unlinked, errs
}

// workspaceOfInvoice menelusuri invoice → SO → company → workspace tertaut. ok=false bila tidak tertaut.
func (a *TenantAccess) workspaceOfInvoice(ctx context.Context, platform coretenant.Scope, inv BillingInvoice) (string, bool, error) {
	soID := ""
	switch {
	case receivabledomain.SourceType(inv.SourceType) == receivabledomain.SourceSalesOrder:
		soID = inv.SourceID
	case inv.ContractID != "":
		info, err := a.contracts.ContractAccess(ctx, platform, inv.ContractID)
		if err != nil {
			return "", false, fmt.Errorf("read contract: %w", err)
		}
		if receivabledomain.SourceType(info.SourceType) == receivabledomain.SourceSalesOrder {
			soID = info.SourceID
		}
	}
	if soID == "" {
		return "", false, nil
	}
	so, err := a.orders.FindByID(ctx, platform, soID)
	if err != nil {
		return "", false, fmt.Errorf("read sales order: %w", err)
	}
	if so.CompanyID == nil {
		return "", false, nil
	}
	company, err := a.companies.FindByID(ctx, platform, *so.CompanyID)
	if err != nil {
		return "", false, fmt.Errorf("read company: %w", err)
	}
	if company.TenantOrganizationID == nil || *company.TenantOrganizationID == "" {
		return "", false, nil
	}
	return *company.TenantOrganizationID, true, nil
}
