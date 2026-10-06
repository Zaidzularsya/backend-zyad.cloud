package service

import (
	"context"
	"log/slog"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/repository"
)

// MarkOverdue menandai invoice issued yang lewat jatuh tempo. today = tanggal
// bisnis (Asia/Jakarta) sebagai date UTC midnight, mis. businesstime.DayOf(now).
func MarkOverdue(ctx context.Context, scope coretenant.Scope, repo repository.InvoiceRepository, today time.Time) (int64, error) {
	return repo.MarkOverdue(ctx, scope, today)
}

// OverdueWorkerIdentity adalah identitas layanan job overdue (cmd/worker) pada WorkerResolver.
const OverdueWorkerIdentity = "receivable-overdue"

const overduePageSize = 100

// OrganizationLister memberi id organisasi aktif secara berhalaman (adapter di internal/app).
type OrganizationLister interface {
	ListActive(ctx context.Context, limit, offset int) ([]string, error)
}

type OverdueResult struct {
	Checked int // organisasi yang dikunjungi
	Marked  int64
	Failed  int // organisasi yang gagal diproses (dicatat, tidak menghentikan putaran)
}

// OverdueRunner menandai invoice jatuh tempo untuk setiap organisasi aktif.
type OverdueRunner struct {
	orgs     OrganizationLister
	scopes   PublicScopeResolver
	invoices repository.InvoiceRepository
	now      func() time.Time
	pageSize int
}

func NewOverdueRunner(orgs OrganizationLister, scopes PublicScopeResolver, invoices repository.InvoiceRepository, now func() time.Time) *OverdueRunner {
	if now == nil {
		now = time.Now
	}
	return &OverdueRunner{orgs: orgs, scopes: scopes, invoices: invoices, now: now, pageSize: overduePageSize}
}

// RunOnce satu putaran: tiap organisasi ditandai dengan tanggal bisnis WIB. Galat per organisasi
// dicatat lalu dilanjutkan; hanya pembatalan konteks atau galat daftar organisasi yang menghentikan putaran.
func (r *OverdueRunner) RunOnce(ctx context.Context) (OverdueResult, error) {
	var res OverdueResult
	today := businesstime.DayOf(r.now())
	for offset := 0; ; offset += r.pageSize {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		ids, err := r.orgs.ListActive(ctx, r.pageSize, offset)
		if err != nil {
			return res, err
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			res.Checked++
			n, err := r.markOrganization(ctx, id, today)
			if err != nil {
				res.Failed++
				slog.WarnContext(ctx, "receivable: overdue run failed for organization", "organization_id", id, "error", err)
				continue
			}
			res.Marked += n
		}
		if len(ids) < r.pageSize {
			return res, nil
		}
	}
}

func (r *OverdueRunner) markOrganization(ctx context.Context, organizationID string, today time.Time) (int64, error) {
	tenantCtx, err := r.scopes.ResolveWorkerOrganization(ctx, organizationID, OverdueWorkerIdentity)
	if err != nil {
		return 0, err
	}
	scope, err := coretenant.NewScope(tenantCtx)
	if err != nil {
		return 0, err
	}
	return MarkOverdue(ctx, scope, r.invoices, today)
}
