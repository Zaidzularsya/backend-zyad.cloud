package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/repository"
)

// MarkOverdue menandai invoice issued yang lewat jatuh tempo. today = tanggal
// bisnis (Asia/Jakarta) sebagai date UTC midnight, mis. businesstime.DayOf(now).
func MarkOverdue(ctx context.Context, scope coretenant.Scope, repo repository.InvoiceRepository, today time.Time) (int64, error) {
	return repo.MarkOverdue(ctx, scope, today)
}
