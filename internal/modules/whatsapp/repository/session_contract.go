package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

type CreateSessionParams struct {
	Name           string
	DisplayName    string
	Engine         string
	Purpose        domain.SessionPurpose
	IsDefault      bool
	AutoCreateLead bool
	CreatedBy      string
}

// UpdateSessionParams applies only non-nil fields.
type UpdateSessionParams struct {
	DisplayName    *string
	IsDefault      *bool
	Purpose        *domain.SessionPurpose
	AutoCreateLead *bool
	UpdatedBy      string
}

// UpdateSessionStatusParams records a status observed from WAHA. Phone and
// PushName are only written when non-nil (known once the session is WORKING).
type UpdateSessionStatusParams struct {
	Status   domain.SessionStatus
	Phone    *string
	PushName *string
	At       time.Time
}

// SessionRepository manages wa_sessions (RLS) together with its
// wa_session_directory row, so both stay consistent in one transaction.
// Lookups and mutations ignore soft-deleted sessions and return
// pgx.ErrNoRows when nothing matches.
type SessionRepository interface {
	// Create inserts the session and its directory entry. When IsDefault is
	// true, the organization's previous default is unset first.
	Create(ctx context.Context, scope coretenant.Scope, params CreateSessionParams) (domain.Session, error)
	GetByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error)
	List(ctx context.Context, scope coretenant.Scope) ([]domain.Session, error)
	// CountActive counts non-deleted sessions, for the whatsapp.max_sessions quota.
	CountActive(ctx context.Context, scope coretenant.Scope) (int64, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateSessionParams) (domain.Session, error)
	// UpdateStatus also returns the status before the update. The row is
	// locked while reading it, so concurrent writers (webhook processor and
	// reconciler) each observe a distinct previous status.
	UpdateStatus(ctx context.Context, scope coretenant.Scope, id string, params UpdateSessionStatusParams) (domain.Session, domain.SessionStatus, error)
	// SoftDelete marks the session and its directory entry deleted and clears is_default.
	SoftDelete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error
}
