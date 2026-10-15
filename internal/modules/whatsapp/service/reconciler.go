package service

import (
	"context"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/repository"
)

// WorkerIdentity is the internal service identity the whatsapp worker uses
// with organization/service.WorkerResolver.
const WorkerIdentity = "whatsapp-worker"

type WorkerTenantResolver interface {
	ResolveWorkerOrganization(ctx context.Context, organizationID string, serviceIdentity string) (coretenant.Context, error)
}

// ReconcileResult summarizes one reconcile pass.
type ReconcileResult struct {
	Checked int
	Failed  int
}

// StatusReconciler periodically re-reads every app-owned session from WAHA,
// catching status changes whose webhook was lost. It only iterates
// wa_session_directory, so sessions created outside the app on the shared
// WAHA server (e.g. n8n's) are never touched.
type StatusReconciler struct {
	directory repository.DirectoryRepository
	resolver  WorkerTenantResolver
	sessions  *SessionService
	log       *slog.Logger
}

func NewStatusReconciler(directory repository.DirectoryRepository, resolver WorkerTenantResolver, sessions *SessionService, log *slog.Logger) *StatusReconciler {
	if log == nil {
		log = slog.Default()
	}
	return &StatusReconciler{directory: directory, resolver: resolver, sessions: sessions, log: log}
}

// RunOnce reconciles every active session. A failure on one session is
// logged and does not stop the others.
func (r *StatusReconciler) RunOnce(ctx context.Context) (ReconcileResult, error) {
	entries, err := r.directory.ListActive(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}

	result := ReconcileResult{}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Checked++
		if err := r.reconcileEntry(ctx, entry.OrganizationID, entry.SessionID); err != nil {
			result.Failed++
			r.log.Warn("whatsapp: reconcile session failed",
				"organization_id", entry.OrganizationID, "session_id", entry.SessionID, "error", err)
		}
	}
	return result, nil
}

func (r *StatusReconciler) reconcileEntry(ctx context.Context, organizationID, sessionID string) error {
	tenantContext, err := r.resolver.ResolveWorkerOrganization(ctx, organizationID, WorkerIdentity)
	if err != nil {
		return err
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		return err
	}
	ctx = coretenant.WithContext(ctx, tenantContext)
	_, err = r.sessions.Reconcile(ctx, scope, sessionID)
	return err
}
