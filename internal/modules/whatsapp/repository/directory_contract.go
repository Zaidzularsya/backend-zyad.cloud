package repository

import (
	"context"

	"zyad.cloud/internal/modules/whatsapp/domain"
)

// DirectoryRepository reads wa_session_directory, which is intentionally not
// under RLS. Only the webhook receiver and the worker may use it, to find the
// organization of a WAHA session before opening a tenant-scoped transaction.
// Never expose it through tenant endpoints.
type DirectoryRepository interface {
	// Resolve returns the active entry for a WAHA session name, or pgx.ErrNoRows.
	Resolve(ctx context.Context, sessionName string) (domain.DirectoryEntry, error)
	// ListActive returns every non-deleted entry across organizations.
	ListActive(ctx context.Context) ([]domain.DirectoryEntry, error)
}
