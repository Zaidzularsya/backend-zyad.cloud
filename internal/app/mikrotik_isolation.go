package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"zyad.cloud/internal/config"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	mailboxservice "zyad.cloud/internal/modules/mailbox/service"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/modules/organization"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database"
)

// MikrotikIsolationWorkerIdentity is the WorkerResolver identity of the isolation job.
const MikrotikIsolationWorkerIdentity = "mikrotik-isolation"

const (
	// mikrotikIsolationBatch is how many tasks one organization processes per tick.
	mikrotikIsolationBatch = 20
	// mikrotikIsolationOrgPage is the page size of the active organization listing.
	mikrotikIsolationOrgPage = 100
	// mikrotikHookTimeout bounds an evaluation run from a request-path hook
	// (invoice paid/voided, pairing, exemption).
	mikrotikHookTimeout = 5 * time.Second

	mikrotikFeatureKey = "mikrotik.enabled"
)

// mikrotikIsolationMode normalizes the configured mode; anything unknown is off.
func mikrotikIsolationMode(raw string) string {
	switch raw {
	case mikrotikservice.ModeOn, mikrotikservice.ModeDryRun:
		return raw
	}
	return mikrotikservice.ModeOff
}

// --- runner (cmd/worker) ---

type isolationScopeResolver interface {
	ResolveWorkerOrganization(ctx context.Context, organizationID, serviceIdentity string) (coretenant.Context, error)
}

type isolationFeatureChecker interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// isolationReadiness tells whether an organization has anything for the
// engine to do: a router able to isolate, or secrets that must be restored.
type isolationReadiness interface {
	HasReadyRouter(ctx context.Context, scope coretenant.Scope) (bool, error)
	HasIsolatedSecrets(ctx context.Context, scope coretenant.Scope) (bool, error)
}

type isolationReconciler interface {
	RunOrganization(ctx context.Context, scope coretenant.Scope) (mikrotikservice.ReconcileResult, error)
	RunOrganizationRestoreOnly(ctx context.Context, scope coretenant.Scope) (mikrotikservice.ReconcileResult, error)
}

type isolationProcessor interface {
	RunOnce(ctx context.Context, scope coretenant.Scope, batch int) (mikrotikservice.ProcessResult, error)
	RunRestoreOnly(ctx context.Context, scope coretenant.Scope, batch int) (mikrotikservice.ProcessResult, error)
}

type mikrotikIsolationRunnerDeps struct {
	Mode       string
	Orgs       receivableservice.OrganizationLister
	Scopes     isolationScopeResolver
	Features   isolationFeatureChecker
	Readiness  isolationReadiness
	Reconciler isolationReconciler
	Processor  isolationProcessor
}

// IsolationRunResult summarizes one RunOnce. It carries counts only, never
// customer data.
type IsolationRunResult struct {
	Organizations  int // organizations processed (full or restore-only)
	RestoreOnly    int // of which processed restore-only (no mikrotik.enabled entitlement)
	Skipped        int // nothing to do (no ready router / no isolated secrets)
	Failed         int // organizations with a resolve/readiness/reconcile/process error
	Accounts       int // accounts evaluated by the reconciler
	AccountsFailed int
	Tasks          mikrotikservice.ProcessResult
}

// Active reports whether the round did anything worth logging.
func (r IsolationRunResult) Active() bool {
	t := r.Tasks
	return r.Failed > 0 || r.AccountsFailed > 0 || t.Done+t.Retried+t.Failed+t.Cancelled+t.Skipped > 0
}

// MikrotikIsolationRunner is the periodic isolation job: per organization it
// runs the reconciler (enqueue) and, in mode "on", the task processor
// (router writes).
type MikrotikIsolationRunner struct {
	d        mikrotikIsolationRunnerDeps
	pageSize int
}

func newMikrotikIsolationRunner(d mikrotikIsolationRunnerDeps) *MikrotikIsolationRunner {
	d.Mode = mikrotikIsolationMode(d.Mode)
	return &MikrotikIsolationRunner{d: d, pageSize: mikrotikIsolationOrgPage}
}

// RunOnce visits every active organization. Per-organization failures are
// logged and counted; only listing errors and ctx cancellation stop the round.
func (r *MikrotikIsolationRunner) RunOnce(ctx context.Context) (IsolationRunResult, error) {
	var res IsolationRunResult
	for offset := 0; ; offset += r.pageSize {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		ids, err := r.d.Orgs.ListActive(ctx, r.pageSize, offset)
		if err != nil {
			return res, err
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			r.runOrganization(ctx, id, &res)
		}
		if len(ids) < r.pageSize {
			return res, nil
		}
	}
}

// runOrganization processes one organization. Without the mikrotik.enabled
// entitlement (or when the check itself errors) the organization runs in
// restore-only mode: losing the entitlement must never isolate anyone, but
// customers already isolated must still be restorable (e.g. after paying).
func (r *MikrotikIsolationRunner) runOrganization(ctx context.Context, orgID string, res *IsolationRunResult) {
	entErr := r.d.Features.RequireFeature(ctx, orgID, mikrotikFeatureKey)
	if entErr != nil && !isFeatureNotEnabled(entErr) {
		slog.DebugContext(ctx, "mikrotik: entitlement check failed, running restore-only", "organization_id", orgID)
	}
	restoreOnly := entErr != nil
	failed := false
	fail := func(step string, err error) {
		if !failed {
			failed = true
			res.Failed++
		}
		slog.WarnContext(ctx, "mikrotik: isolation run failed for organization", "organization_id", orgID, "step", step, "error", err)
	}
	tc, err := r.d.Scopes.ResolveWorkerOrganization(ctx, orgID, MikrotikIsolationWorkerIdentity)
	if err != nil {
		fail("resolve", err)
		return
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		fail("scope", err)
		return
	}
	ready := false
	if !restoreOnly {
		if ready, err = r.d.Readiness.HasReadyRouter(ctx, scope); err != nil {
			fail("readiness", err)
			return
		}
	}
	if !ready {
		// Restores never depend on isolir_ready: an organization whose routers
		// lost readiness must still get its isolated customers back.
		isolated, err := r.d.Readiness.HasIsolatedSecrets(ctx, scope)
		if err != nil {
			fail("readiness", err)
			return
		}
		if !isolated {
			res.Skipped++
			return
		}
	}
	res.Organizations++
	reconcile, process := r.d.Reconciler.RunOrganization, r.processRunOnce
	if restoreOnly {
		res.RestoreOnly++
		reconcile, process = r.d.Reconciler.RunOrganizationRestoreOnly, r.processRestoreOnly
	}

	rec, recErr := reconcile(ctx, scope)
	res.Accounts += rec.Accounts
	res.AccountsFailed += rec.Failed
	if recErr != nil {
		// Pending tasks are independent of this pass: still process them below.
		fail("reconcile", recErr)
	}

	// dry_run never touches routers. Tasks left pending from an earlier "on"
	// period stay untouched on purpose until the mode is "on" again.
	if r.d.Mode != mikrotikservice.ModeOn || r.d.Processor == nil {
		return
	}
	proc, err := process(ctx, scope)
	res.Tasks.Done += proc.Done
	res.Tasks.Retried += proc.Retried
	res.Tasks.Failed += proc.Failed
	res.Tasks.Cancelled += proc.Cancelled
	res.Tasks.Skipped += proc.Skipped
	if err != nil {
		fail("process", err)
	}
}

func (r *MikrotikIsolationRunner) processRunOnce(ctx context.Context, scope coretenant.Scope) (mikrotikservice.ProcessResult, error) {
	return r.d.Processor.RunOnce(ctx, scope, mikrotikIsolationBatch)
}

func (r *MikrotikIsolationRunner) processRestoreOnly(ctx context.Context, scope coretenant.Scope) (mikrotikservice.ProcessResult, error) {
	return r.d.Processor.RunRestoreOnly(ctx, scope, mikrotikIsolationBatch)
}

// isFeatureNotEnabled reports the plain "feature not enabled" denial of
// EntitlementGuard, as opposed to an infrastructure error.
func isFeatureNotEnabled(err error) bool {
	var appErr *coreerrors.AppError
	return errors.As(err, &appErr) && appErr.Code == organization.ErrCodeFeatureNotEnabled
}

// mikrotikReadiness adapts the router and secret repositories to isolationReadiness.
type mikrotikReadiness struct {
	routers mikrotikrepo.RouterRepository
	secrets mikrotikrepo.SecretRepository
}

func (m mikrotikReadiness) HasReadyRouter(ctx context.Context, scope coretenant.Scope) (bool, error) {
	return m.routers.HasReadyRouter(ctx, scope)
}

func (m mikrotikReadiness) HasIsolatedSecrets(ctx context.Context, scope coretenant.Scope) (bool, error) {
	ids, err := m.secrets.AccountsWithIsolatedSecrets(ctx, scope)
	return len(ids) > 0, err
}

// NewMikrotikIsolationRunner builds the isolation job for cmd/worker. It
// returns nil, nil when the mode is off (the job does not exist then). Like
// the billing run it returns an error when tenant storage is not configured,
// because the customer notifier reuses the worker receivable module.
func NewMikrotikIsolationRunner(cfg config.Config, db *database.Pool, log *slog.Logger) (*MikrotikIsolationRunner, error) {
	mode := mikrotikIsolationMode(cfg.Mikrotik.IsolationMode)
	if mode == mikrotikservice.ModeOff {
		return nil, nil
	}
	b, err := newWorkerReceivableBuild(cfg, db, log)
	if err != nil {
		return nil, err
	}
	module := buildReceivable(b)

	routerRepo := mikrotikrepo.NewRouterRepository(db)
	secretRepo := mikrotikrepo.NewSecretRepository(db)
	taskRepo := mikrotikrepo.NewTaskRepository(db)
	source := newMikrotikReceivableReader(db)
	routers := mikrotikservice.NewRouterService(routerRepo, mikrotikservice.NewPlatformClientFactory(),
		cfg.App.Secret, cfg.Mikrotik.AllowedPrivateCIDRs)
	eval := mikrotikservice.NewEvaluator(mikrotikservice.EvaluatorDeps{
		Source: source, Policy: mikrotikrepo.NewPolicyRepository(db), Secrets: secretRepo, Tasks: taskRepo, Mode: mode,
	})
	members := receivableMembers{members: b.Members, perms: b.Permissions}
	processor := mikrotikservice.NewTaskProcessor(mikrotikservice.TaskProcessorDeps{
		Tasks: taskRepo, Secrets: secretRepo, Routers: routers, Evaluator: eval,
		Executor: mikrotikservice.NewExecutor(routers), Locker: mikrotikrepo.NewAdvisoryLocker(db),
		Customer: &mikrotikCustomerNotifier{
			settings: receivablerepo.NewSettingsRepository(db), accounts: module.Accounts, links: module.Invoices,
			email:    mailboxservice.NewInvoiceMailer(b.Mailboxes, b.Messages),
			whatsapp: whatsappservice.NewInvoiceSender(b.WhatsApp, b.Entitlements),
			issuer:   receivableIssuer{repo: crmrepo.NewQuotationIssuerRepository(db)},
			now:      time.Now,
		},
		Alert: mikrotikTenantAlerter{
			members: members, publisher: b.Publisher, appName: cfg.App.Name,
			frontendURL: cfg.App.FrontendURL, locale: cfg.Notification.DefaultLocale,
		},
		Audit: routerRepo,
	})
	return newMikrotikIsolationRunner(mikrotikIsolationRunnerDeps{
		Mode:       mode,
		Orgs:       receivableOrgLister{repo: organizationrepo.NewOrganizationRepository(db)},
		Scopes:     organizationservice.NewWorkerResolver(organizationrepo.NewOrganizationRepository(db), MikrotikIsolationWorkerIdentity),
		Features:   b.Entitlements,
		Readiness:  mikrotikReadiness{routers: routerRepo, secrets: secretRepo},
		Reconciler: mikrotikservice.NewReconciler(source, secretRepo, eval, nil),
		Processor:  processor,
	}), nil
}

// --- API wiring: event hooks ---

// detachedHookContext keeps request values but not the request's
// cancellation: hooks run after the change committed, so a client hanging up
// must not abort the evaluation. The timeout bounds the request path.
func detachedHookContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// mikrotikEvaluateHook adapts Evaluator.Apply to the PolicyDeps.OnChange /
// SecretService.SetOnChange hook signature. Errors are logged only: the
// reconciler is the safety net.
func mikrotikEvaluateHook(eval *mikrotikservice.Evaluator, event string) func(ctx context.Context, scope coretenant.Scope, accountID string) {
	return func(ctx context.Context, scope coretenant.Scope, accountID string) {
		hctx, cancel := detachedHookContext(ctx, mikrotikHookTimeout)
		defer cancel()
		if _, err := eval.Apply(hctx, scope, accountID, event); err != nil {
			slog.WarnContext(hctx, "mikrotik: evaluate account after change failed", "account_id", accountID, "event", event, "error", err)
		}
	}
}

type mikrotikInvoiceChanges interface {
	OnInvoiceChanged(ctx context.Context, scope coretenant.Scope, invoiceID, event string)
}

// mikrotikReceivableListener forwards receivable invoice events to the
// mikrotik InvoiceListener (the mikrotik module does not import receivable).
type mikrotikReceivableListener struct {
	changes mikrotikInvoiceChanges
	timeout time.Duration
}

func (l mikrotikReceivableListener) forward(ctx context.Context, scope coretenant.Scope, invoiceID, event string) {
	hctx, cancel := detachedHookContext(ctx, l.timeout)
	defer cancel()
	l.changes.OnInvoiceChanged(hctx, scope, invoiceID, event)
}

func (l mikrotikReceivableListener) InvoicePaid(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.forward(ctx, scope, inv.ID, "invoice_paid")
}

// InvoicePaidReplay is safe to repeat: the evaluator enqueues at most one
// active task per secret.
func (l mikrotikReceivableListener) InvoicePaidReplay(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.forward(ctx, scope, inv.ID, "invoice_paid_replay")
}

func (l mikrotikReceivableListener) InvoiceVoided(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.forward(ctx, scope, inv.ID, "invoice_voided")
}

func (mikrotikReceivableListener) ContractCreated(context.Context, coretenant.Scope, receivableservice.ContractRef) {
}

func (mikrotikReceivableListener) ContractEnded(context.Context, coretenant.Scope, receivableservice.ContractRef) {
}

// registerMikrotikInvoiceListener adds the listener only when the engine is
// not off, so mode off leaves the receivable flows exactly as before. It
// reports whether it registered.
func registerMikrotikInvoiceListener(mode string, reg *receivableservice.Registry, changes mikrotikInvoiceChanges) bool {
	if mikrotikIsolationMode(mode) == mikrotikservice.ModeOff || reg == nil || changes == nil {
		return false
	}
	reg.Add(mikrotikReceivableListener{changes: changes, timeout: mikrotikHookTimeout})
	return true
}
