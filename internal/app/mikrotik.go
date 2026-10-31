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
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/database"
)

// mikrotikModule groups the handlers built by buildMikrotik.
type mikrotikModule struct {
	Router *mikrotikhandler.RouterHandler
	Setup  *mikrotikhandler.SetupHandler
	Secret *mikrotikhandler.SecretHandler
	Policy *mikrotikhandler.PolicyHandler
}

// mikrotikBuild carries the cross-module services the adapters wrap.
type mikrotikBuild struct {
	Accounts mikrotikAccountSource
	Contacts mikrotikContactCreator
	Products mikrotikProductSource
	// Listeners is the receivable registry; the isolation engine registers
	// its invoice listener there when IsolationMode is not off.
	Listeners *receivableservice.Registry
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
	if mode := mikrotikIsolationMode(cfg.Mikrotik.IsolationMode); mode != mikrotikservice.ModeOff {
		eval := mikrotikservice.NewEvaluator(mikrotikservice.EvaluatorDeps{
			Source: reader, Policy: policyRepo, Secrets: secretRepo, Tasks: mikrotikrepo.NewTaskRepository(db), Mode: mode,
		})
		policyDeps.OnChange = mikrotikEvaluateHook(eval, "customer_policy_changed")
		secrets.SetOnChange(mikrotikEvaluateHook(eval, "secret_pairing_changed"))
		registerMikrotikInvoiceListener(mode, b.Listeners, mikrotikservice.NewInvoiceListener(reader, eval))
	}
	policy := mikrotikservice.NewPolicyService(policyDeps)
	return mikrotikModule{
		Router: mikrotikhandler.NewRouterHandler(svc),
		Setup:  mikrotikhandler.NewSetupHandler(setup),
		Secret: mikrotikhandler.NewSecretHandler(secrets),
		Policy: mikrotikhandler.NewPolicyHandler(policy),
	}
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
}
