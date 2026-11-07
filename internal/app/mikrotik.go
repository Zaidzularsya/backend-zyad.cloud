package app

import (
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/database"
)

// mikrotikModule groups the handlers built by buildMikrotik.
type mikrotikModule struct {
	Router *mikrotikhandler.RouterHandler
	Setup  *mikrotikhandler.SetupHandler
	Secret *mikrotikhandler.SecretHandler
	Policy *mikrotikhandler.PolicyHandler
	Task   *mikrotikhandler.TaskHandler
	// Onboarding serves the setup checklist and per-customer isolir status.
	Onboarding *mikrotikhandler.OnboardingHandler
	// Control serves the tenant control center (mode, grace days, pilot, restore all).
	Control *mikrotikhandler.ControlHandler
	// Preview serves the isolation candidate preview and its CSV export.
	Preview *mikrotikhandler.PreviewHandler
	// Activity serves the control center activity log.
	Activity *mikrotikhandler.ActivityHandler
	// Public serves the unauthenticated /public/isolir/* page; mount it ONLY on
	// the host-resolved public group.
	Public *mikrotikhandler.IsolirPublicHandler
}

// mikrotikBuild carries the cross-module services the adapters wrap.
type mikrotikBuild struct {
	Accounts mikrotikAccountSource
	Contacts mikrotikContactCreator
	Products mikrotikProductSource
	// Listeners is the receivable registry; the isolation engine registers
	// its invoice listener there when IsolationMode is not off.
	Listeners *receivableservice.Registry
	// Public isolir page collaborators (nil-safe until the page is hit).
	PayLinks    payLinker
	Issuer      mikrotikNotifIssuer
	Features    mikrotikPublicFeatures
	RateCounter middleware.RateCounter
}

// buildMikrotik assembles the Mikrotik module: repositories, production
// RouterOS client factory, services, cross-module adapters and HTTP handlers.
func buildMikrotik(cfg config.Config, db *database.Pool, b mikrotikBuild) mikrotikModule {
	repo := mikrotikrepo.NewRouterRepository(db)
	svc := mikrotikservice.NewRouterService(
		repo,
		mikrotikservice.NewPlatformClientFactory(),
		cfg.App.Secret,
		cfg.Mikrotik.AllowedPrivateCIDRs,
	)
	setup := mikrotikservice.NewSetupService(svc, repo, mikrotikrepo.NewAdvisoryLocker(db),
		mikrotikservice.SetupOptions{AllowREST: cfg.Mikrotik.SetupAllowREST})

	reader := newMikrotikReceivableReader(db)
	accounts := mikrotikAccountDirectory{accounts: b.Accounts}
	secretRepo := mikrotikrepo.NewSecretRepository(db)
	policyRepo := mikrotikrepo.NewPolicyRepository(db)
	secrets := mikrotikservice.NewSecretService(svc, secretRepo, repo, accounts,
		mikrotikContactProvisioner{contacts: b.Contacts, accounts: b.Accounts})
	policyDeps := mikrotikservice.PolicyDeps{
		Repo:     policyRepo,
		Accounts: accounts,
		Products: mikrotikProductCatalog{products: b.Products, inUse: reader},
		Usage:    reader,
		Audit:    repo,
	}

	// Isolation engine triggers (API process only: payments and pairing
	// happen here; the worker runs the reconciler and the task processor).
	// Mode off wires nothing, so existing flows are unchanged. All hooks are
	// set before serving.
	engineMode := mikrotikIsolationMode(cfg.Mikrotik.IsolationMode)
	controlRepo := mikrotikrepo.NewControlRepository(db)
	taskRepo := mikrotikrepo.NewTaskRepository(db)
	var eval *mikrotikservice.Evaluator
	if engineMode != mikrotikservice.ModeOff {
		eval = mikrotikservice.NewEvaluator(mikrotikservice.EvaluatorDeps{
			Source: reader, Policy: policyRepo, Secrets: secretRepo, Tasks: taskRepo, Mode: engineMode,
			Control: controlRepo,
		})
		policyDeps.OnChange = mikrotikEvaluateHook(eval, "customer_policy_changed")
		secrets.SetOnChange(mikrotikEvaluateHook(eval, "secret_pairing_changed"))
		registerMikrotikInvoiceListener(engineMode, b.Listeners, mikrotikservice.NewInvoiceListener(reader, eval))
	}
	policy := mikrotikservice.NewPolicyService(policyDeps)
	preview := mikrotikservice.NewPreviewService(reader, controlRepo, nil)
	onboarding := mikrotikservice.NewOnboardingService(
		mikrotikrepo.NewOnboardingRepository(db), mikrotikSenderSettings{settings: receivablerepo.NewSettingsRepository(db)})
	onboarding.SetPendingReader(preview)
	publicAdapter := newMikrotikPublicAdapter(secretRepo, reader, b.Accounts,
		receivablerepo.NewSettingsRepository(db), b.PayLinks, b.Issuer, b.Features)
	publicSvc := mikrotikservice.NewPublicService(mikrotikservice.PublicServiceDeps{
		Directory: publicAdapter, Secrets: secretRepo, Overdue: reader, PayLinks: publicAdapter,
		Profile: publicAdapter, Gate: publicAdapter, Tasks: taskRepo,
	})
	return mikrotikModule{
		Public: mikrotikhandler.NewIsolirPublicHandler(publicSvc, b.RateCounter),
		Router: mikrotikhandler.NewRouterHandler(svc),
		Setup:  mikrotikhandler.NewSetupHandler(setup),
		Secret: mikrotikhandler.NewSecretHandler(mikrotikSecretFacade{
			SecretService: secrets,
			SuggestionService: mikrotikservice.NewSuggestionService(svc, secretRepo, accounts,
				mikrotikrepo.NewSuggestionRepository(db), mikrotikrepo.NewAdvisoryLocker(db)),
		}),
		Policy:     mikrotikhandler.NewPolicyHandler(policy),
		Task:       mikrotikhandler.NewTaskHandler(mikrotikservice.NewTaskService(taskRepo, secretRepo, repo)),
		Onboarding: mikrotikhandler.NewOnboardingHandler(onboarding),
		// eval is nil while the engine is off; the service then refuses pilot tests.
		Control: mikrotikhandler.NewControlHandler(mikrotikservice.NewControlService(mikrotikservice.ControlDeps{
			Control: controlRepo, Secrets: secretRepo, Tasks: taskRepo, Accounts: accounts, Audit: repo, Evaluator: eval,
		}, engineMode)),
		Preview: mikrotikhandler.NewPreviewHandler(preview),
		Activity: mikrotikhandler.NewActivityHandler(mikrotikservice.NewActivityService(
			mikrotikrepo.NewActivityRepository(db), reader)),
	}
}

// mikrotikSecretFacade presents the secret and suggestion services as the one
// service interface the secret handler expects.
type mikrotikSecretFacade struct {
	*mikrotikservice.SecretService
	*mikrotikservice.SuggestionService
}

// Every call that logs in to a tenant router (create, update, check, setup/*)
// is limited per organization: each one is a login attempt against a
// tenant-chosen host (credential oracle, the router may blacklist the platform
// IP) and can hold a socket for many seconds.
const (
	mikrotikProbeLimit       = 20
	mikrotikProbeLimitWindow = time.Minute
)

// registerMikrotikRoutes mounts /app/mikrotik under the authenticated group.
// Tenant rules match CRM/receivable and the whole group is gated by the
// mikrotik.enabled entitlement. Order per route: tenant + entitlement guards,
// then the per-organization rate limit (router-contacting routes only), then
// the permission check. Kept separate from newRouter so the gates can be tested
// without the full dependency graph.
func registerMikrotikRoutes(protected *gin.RouterGroup, deps Dependencies) {
	if deps.MikrotikRouterHandler == nil {
		return
	}
	group := protected.Group("/app/mikrotik")
	group.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerOrPlatformTenant(),
		middleware.RequireEntitlement(deps.CRMEntitlementChecker, "mikrotik.enabled"),
	)
	// Fail open when no counter is configured or Redis errors (middleware behavior).
	probeLimit := middleware.RateLimit(deps.MikrotikRateCounter, "rl:mikrotik:org:", mikrotikProbeLimit, mikrotikProbeLimitWindow,
		func(c *gin.Context) string {
			if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
				return tc.OrganizationID()
			}
			return "unknown"
		})
	deps.MikrotikRouterHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	if deps.MikrotikSetupHandler != nil {
		deps.MikrotikSetupHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikSecretHandler != nil {
		deps.MikrotikSecretHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikPolicyHandler != nil {
		deps.MikrotikPolicyHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikTaskHandler != nil {
		deps.MikrotikTaskHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikOnboardingHandler != nil {
		deps.MikrotikOnboardingHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikControlHandler != nil {
		deps.MikrotikControlHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikPreviewHandler != nil {
		deps.MikrotikPreviewHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
	if deps.MikrotikActivityHandler != nil {
		deps.MikrotikActivityHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
}
