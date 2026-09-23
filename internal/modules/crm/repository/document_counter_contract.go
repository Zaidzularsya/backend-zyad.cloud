package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
)

// DocumentCounterRepository hands out the next sequence number for a
// tenant-scoped document type (e.g. "quotation", "invoice") within the
// current calendar year — used to auto-generate human-readable numbers like
// QUO-2026-0001. Counters reset per organization+document_type+year.
type DocumentCounterRepository interface {
	NextNumber(ctx context.Context, scope coretenant.Scope, documentType string) (int, error)
}
