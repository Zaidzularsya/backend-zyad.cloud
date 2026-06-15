# Multi-Tenant Development Tasks

Dokumen ini adalah breakdown development capability multi-tenant lintas platform.

Sources:

- `docs/reference-multi-tenant.md`
- `README.md`
- `docs/reference-auth-user.md`
- `docs/reference-landing-page.md`
- `docs/migration-guide.md`

## Ownership

| Concern | Location |
| --- | --- |
| Organization lifecycle/membership/domain/entitlement business rules | `internal/modules/organization` |
| Tenant context and HTTP middleware | `internal/core/middleware` |
| Auth session/claims integration | `internal/core/auth`, `internal/modules/user` |
| Permission evaluation | `internal/core/permission` |
| Database routing/RLS transaction helper | `internal/platform/database` |
| Tenant-scoped storage | `internal/platform/storage` |
| App wiring | `internal/app` |

Do not create a second business module named `tenant`. `organization` is the canonical tenant module.

## Phase 0 - Contract and Audit

### MT-0001: Canonical Terminology and Context Contract

Status: `done`

Scope:

- Define `organization` as canonical business tenant.
- Define platform organization and customer organization.
- Define `TenantContext` fields and context APIs.
- Define canonical `X-Organization-ID` header and compatibility aliases.
- Define fail-closed behavior.

Acceptance:

- No ambiguity between tenant and organization in new code.
- Platform organization is represented by a real organization ID.
- Context cannot be constructed from untrusted request data alone.

### MT-0002: Current Schema and Query Audit

Status: `done`

Output:

- `docs/multi-tenant-schema-query-audit.md`

Scope:

- Inventory tables as global, organization-scoped, or user-global.
- Inventory queries that accept/omit `organization_id`.
- Audit `organization_id NULL` semantics.
- Audit baseline `owner_type` tables.
- Produce cleanup/backfill plan.

Acceptance:

- Every active table has ownership classification.
- High-risk unscoped queries are listed before migration.

### MT-0003: Config and Trusted Proxy Contract

Status: `done`

Output:

- `internal/config` multi-tenant config and production validation.
- `.env.example` platform organization/domain/proxy contract.
- Gin trusted proxy configuration in `internal/app/router.go`.

Scope:

- Platform primary domain and reserved subdomains.
- Header alias transition.
- Trusted proxy CIDR/forwarded host policy.
- Default data placement and platform organization seed config.

Acceptance:

- Host resolution does not trust arbitrary forwarded headers.
- Production validates required platform domain/config.

## Phase 1 - Database Foundation

### MT-DB-001: Organizations

Status: `done`

Migration:

- `migrations/000013_create_organizations.up.sql`
- `migrations/000013_create_organizations.down.sql`

Scope:

- Create `organizations`.
- Fields: type, slug, name, status, timezone, locale, region, data placement, metadata, timestamps, soft delete.
- Unique normalized slug.
- Protect platform organization from normal deletion.

Acceptance:

- Up/down migration exists.
- Status/type/data placement constraints exist.
- Platform and customer type supported.

### MT-DB-002: Organization Memberships

Status: `done`

Migration:

- `migrations/000014_create_organization_memberships.up.sql`
- `migrations/000014_create_organization_memberships.down.sql`

Scope:

- Create `organization_memberships`.
- User, organization, status, ownership flag, membership version, invitation metadata.
- Normalize existing `user_roles.organization_id` relationship.

Acceptance:

- User may join multiple organizations.
- Active membership unique by user/organization.
- Owner lookup is indexed; last-owner enforcement remains a transaction rule in `MT-BE-011`.

### MT-DB-003: Organization Domains

Status: `done`

Migration:

- `migrations/000015_create_organization_domains.up.sql`
- `migrations/000015_create_organization_domains.down.sql`

Scope:

- Create `organization_domains`.
- Domain type, canonical host, status, verification challenge hash, primary flag, SSL status.
- Reserved subdomain registry.

Acceptance:

- Canonical host globally unique.
- Verification secret stored hashed.
- One primary active host per organization/type policy.
- Reserved labels are synchronized from config during `MT-PLAT-002`; the schema rejects claim/reservation conflicts.

### MT-DB-004: Entitlements and Usage

Status: `done`

Migration:

- `migrations/000016_create_organization_entitlements.up.sql`
- `migrations/000016_create_organization_entitlements.down.sql`

Scope:

- Create `organization_entitlements`.
- Optional `organization_usage_counters`.
- Feature key, source, status, limits JSONB, effective window, version.

Acceptance:

- Active entitlement lookup indexed.
- Override is auditable and time-bounded.
- Usage counters use explicit periods and remain a materialized counter, not the authoritative event source.

### MT-DB-005: Active Context and Impersonation

Status: `done`

Migration:

- `migrations/000017_add_session_organization_context.up.sql`
- `migrations/000017_add_session_organization_context.down.sql`

Scope:

- Add active organization/membership version to sessions or session metadata.
- Create `organization_impersonation_sessions` if impersonation enters scope.

Acceptance:

- Session context can be invalidated after membership change.
- Impersonation records operator, target, reason, expiry, and stop time.
- Runtime stale-version comparison and token rotation remain in `MT-CORE-002` and `MT-CORE-006`.

### MT-DB-006: Tenant Audit Metadata

Status: `done`

Migration:

- `migrations/000018_add_tenant_audit_metadata.up.sql`
- `migrations/000018_add_tenant_audit_metadata.down.sql`

Scope:

- Add organization/effective actor/impersonation fields to audit logs where missing.
- Index organization and time.

Acceptance:

- Cross-tenant action can be reconstructed.
- Audit records survive member removal.

### MT-DB-007: RLS Foundation

Scope:

- Create helper convention for `app.organization_id`.
- Enable RLS on first tenant-owned tables.
- Separate runtime and migration DB role requirements.

Acceptance:

- Missing context fails closed.
- Runtime role cannot bypass RLS.
- Integration test proves cross-tenant row denial.

## Phase 2 - Organization Domain

### MT-BE-001: Organization Models

Status: `done`

Progress:

- Organization, membership, domain, entitlement, impersonation session, and typed tenant context models exist.
- Organization, membership, domain, entitlement, and usage alignment is covered by migrations `000013` through `000016`.
- Auth session organization context and impersonation persistence are covered by migration `000017`.
- Enum and behavior alignment is covered by model and tenant context unit tests.

Scope:

- Organization type/status/data placement.
- Membership/status.
- Domain/status/type.
- Entitlement/status/source.
- Tenant context value object.

Acceptance:

- Enums align with migration.
- Domain models do not depend on Gin/SQL driver.

### MT-BE-002: Organization DTO

Status: `done`

Progress:

- Platform organization lifecycle and list request/response DTOs exist.
- Current organization, membership, invitation, ownership transfer, and switch DTOs exist.
- Domain challenge/status and entitlement/usage DTOs exist.
- Tenant mutation request tests reject an exposed `OrganizationID` field.

Scope:

- Platform admin organization CRUD/status.
- Self organization update.
- Membership/invitation/switch.
- Domain and entitlement responses.

Acceptance:

- Internal credential/routing details are not exposed.
- Organization ID is not mass-assignable on tenant-owned resources.

### MT-REPO-001: Organization Repository

Status: `done`

Progress:

- Create, list, detail, update, status, archive, slug lookup, and platform lookup are implemented.
- Slugs are canonicalized before persistence and lookup.
- Default detail/list queries exclude archived or soft-deleted organizations.
- Unit and PostgreSQL integration tests cover lifecycle and canonical slug uniqueness.

Scope:

- Create/list/detail/update/status/archive.
- Find by slug/ID.
- Platform organization lookup.

Acceptance:

- Slug canonicalization and uniqueness handled.
- Archived organizations excluded by default.

### MT-REPO-002: Membership Repository

Status: `done`

Progress:

- Create, detail, active lookup, status/version update, removal, and owner assignment are implemented.
- User organization and organization member lists are implemented with removed memberships excluded by default.
- Ownership transfer and owner-reducing mutations serialize on the organization row.
- PostgreSQL integration tests prove stale membership versions are rejected and concurrent owner removal retains one active owner.
- Session invalidation after membership mutation remains a service responsibility in `MT-BE-011`.

Scope:

- Membership CRUD/status/version.
- List user organizations and organization members.
- Owner count lock/check.

Acceptance:

- Concurrent owner removal cannot leave zero owners.
- Active membership lookup is indexed.

### MT-REPO-003: Domain Repository

Status: `done`

Progress:

- Organization-scoped create, detail, list, challenge reset, verification, activation, primary selection, SSL state, reassignment, and delete operations are implemented.
- Canonical hosts are normalized before persistence and lookup.
- Public host resolution requires an active verified domain and an active non-deleted organization.
- Reassignment always resets verification, primary, attempt, and SSL state before the host can become active again.
- PostgreSQL integration tests cover duplicate hosts, reserved subdomains, suspended organizations, and reassignment isolation.

Scope:

- Domain CRUD, challenge, verification/status, host resolution.

Acceptance:

- Host resolution only returns active verified organization.
- Reassignment cannot bypass verification.

### MT-REPO-004: Entitlement Repository

Status: `done`

Progress:

- Plan, addon, trial, and platform override source upsert is implemented with per-source advisory locking.
- Effective lookup applies status and time windows with deterministic precedence: `platform_override > addon > trial > plan`.
- Row count, maximum version, and latest update metadata are available for cache invalidation.
- Limits are persisted and decoded as structured JSON objects.
- Period usage lookup and atomic increment with an optional hard limit are implemented.
- PostgreSQL integration tests cover the full precedence chain, version updates, and concurrent quota enforcement.

Scope:

- Effective entitlement lookup.
- Upsert plan/addon/trial/override.
- Version and cache invalidation metadata.

Acceptance:

- Time window and precedence deterministic.
- Limits remain structured and validated.

### MT-BE-010: Organization Lifecycle Service

Status: `done`

Implementation:

- Global permission evaluation only reads global role and direct permission assignments with `organization_id IS NULL`.
- Organization permission evaluation uses an explicit organization ID from verified tenant context and only reads assignments for that organization.
- Global and organization evaluation share role grants and direct allow/deny precedence without mixing scopes.
- `RequireOrganization` fails closed without authenticated user or verified tenant context and passes the trusted organization ID to the permission service.
- Auth login, refresh, and current-user permission payloads are restricted to global permissions because those flows do not carry verified organization context.
- Unit tests cover cross-organization role isolation, scoped deny behavior, and missing-context middleware rejection.
- PostgreSQL integration tests cover global/Organization A/Organization B isolation and auth permission payload filtering.

Progress:

- Customer organization creation transactionally persists the `provisioning` organization, active owner membership, initial plan entitlements, default metadata, and audit record.
- Failed owner or entitlement persistence rolls back the complete organization bundle.
- Lifecycle transitions validate the state machine and reject stale status snapshots.
- Suspend, disable, and archive revoke active sessions for the organization in the same transaction.
- Platform organization disable/archive is rejected at both service and transaction boundaries.
- Generic provisioning business event publication remains pending `MT-INFRA-003`; the existing notification outbox is intentionally not reused as a business-event outbox.
- Public-site, background-job, and route-level status enforcement are completed in `MT-CORE-003..005`.

Scope:

- Create organization, owner membership, default settings, plan, and provisioning event transactionally.
- Activate, suspend, disable, restore, archive.
- Platform organization protection.

Acceptance:

- Partial organization creation rolls back or enters explicit provisioning failure.
- Status effects are enforced consistently.

### MT-BE-011: Membership Service

Status: `done`

Progress:

- Invitation creates or refreshes an invited membership for an existing active/pending identity, stores only the token hash, validates organization roles, and writes the tenant audit record atomically.
- Invitation acceptance validates the intended user and expiry before activating the membership and clearing the reusable token data.
- Membership listing reuses the tenant-scoped repository filter and pagination contract.
- Suspend, restore, remove, organization-role synchronization, and ownership transfer run in organization-locked transactions; removal also clears organization role assignments.
- Role changes increment membership version through the database trigger and revoke sessions bound to the changed membership.
- Suspend/remove and ownership transfer revoke both active sessions and their refresh tokens in the same transaction.
- The last active owner guard is enforced under the organization lock.
- Unit tests cover validation, token hashing/normalization, and error mapping. Integration coverage verifies invitation, acceptance, role changes, session invalidation, ownership transfer, audit constraints, and the last-owner guard.

Scope:

- Invite/accept/list/suspend/remove member.
- Transfer ownership.
- Role assignment integration.
- Invalidate sessions on membership changes.

Acceptance:

- Removed/suspended member loses access immediately.
- Last owner cannot be removed.

### MT-BE-012: Entitlement Service

Status: `done`

Progress:

- Effective feature evaluation uses repository precedence and independently revalidates organization, feature, status, and effective window before enabling access.
- Missing, mismatched, inactive, or expired entitlement fails closed; permission checks remain outside the entitlement service.
- Usage checks return zero for an unused explicit period and calculate hard-limit remaining values from structured entitlement limits.
- Usage consumption passes the resolved hard limit to the repository atomic increment and maps quota exhaustion to a stable application error.
- Invalid quota periods, deltas, and non-integer or negative limits are rejected before persistence.
- Unit tests cover expired and missing entitlement, empty usage, hard-limit propagation, quota exhaustion, and malformed limits.

Scope:

- Evaluate enabled feature and limits.
- Apply plan/addon/trial/override precedence.
- Consume/check quota.

Acceptance:

- Permission and entitlement remain separate checks.
- Expired entitlement fails closed.

## Phase 3 - Tenant Context and Resolution

### MT-CORE-001: Tenant Context API

Status: `done`

Progress:

- Typed immutable context and standard/Gin context helpers exist.
- Authenticated context requires active membership identity and positive membership version.
- `RequireContext` and `RequireTenantContext` return explicit errors when context is missing.
- Membership default is represented by an explicit resolution source rather than overloading session resolution.
- Legacy string context remains for compatibility.
- Route-level active/platform/customer enforcement is completed in `MT-CORE-005`.

Scope:

- Replace string-only tenant context with typed immutable `TenantContext`.
- Add Gin and standard context helpers.
- Preserve temporary `TenantID()` compatibility only during migration.

Acceptance:

- Service/repository can read typed context.
- Missing/invalid context has explicit error.

### MT-CORE-002: Authenticated Organization Resolver

Status: `done`

Progress:

- Protected routes run authenticated organization resolution after access-token authentication.
- Canonical and legacy header selectors remain untrusted until an active user membership and active organization are loaded.
- Session organization context verifies membership ID and version snapshot; stale snapshots are rejected.
- A single active membership becomes the default context only when no session context or explicit selector exists.
- Users with no membership or multiple memberships and no active session continue without tenant context so global endpoints remain available; tenant route groups fail closed through `MT-CORE-005`.
- Unit tests cover forged selectors, stale snapshots, single-membership default, and middleware propagation.
- PostgreSQL integration coverage verifies session/header resolution and stale membership version rejection.

Scope:

- Resolve active organization from session.
- Allow canonical header override after membership verification.
- Support single-membership default.
- Verify organization and membership status/version.

Acceptance:

- Forged organization header is rejected.
- User cannot switch into organization without active membership.

### MT-CORE-003: Public Host Resolver

Status: `done`

Progress:

- Request host normalization lowercases DNS names, removes a valid port/trailing dot, and rejects schemes, paths, IP literals, ambiguous forwarded lists, malformed labels, and control/whitespace injection.
- Platform primary domain resolves only to the configured active platform organization.
- Managed subdomain and custom domain resolution reuse the verified active domain registry and require an active organization.
- Public requests cannot select organization context through `X-Organization-ID` or legacy tenant headers.
- `X-Forwarded-Host` is considered only when enabled and the direct remote address belongs to a configured trusted proxy CIDR.
- Public host middleware is wired only to the public module route group; authenticated routes continue using session/header membership resolution.
- Unit tests cover normalization, platform/custom resolution, unknown/disabled hosts, trusted proxy handling, and selector rejection.
- PostgreSQL integration coverage verifies active custom-domain resolution and immediate failure after organization suspension.
- Resolver intentionally performs authoritative database lookup. Versioned Redis caching and invalidation remain centralized in `MT-INFRA-001` to avoid stale suspended/disabled tenant access.

Scope:

- Normalize host.
- Resolve platform primary host, managed subdomain, and verified custom domain.
- Trust forwarded host only from trusted proxy.
- Cache with version/invalidation.

Acceptance:

- Platform marketing host resolves platform organization.
- Host injection and disabled domain are rejected.

### MT-CORE-004: Worker/Internal Resolver

Status: `done`

Progress:

- Worker resolver requires a valid event `organization_id` and an explicitly allowlisted internal service identity.
- Organization lookup validates active lifecycle state and rejects unknown, suspended, disabled, or archived organizations.
- Shared data placement is accepted; dedicated placement fails with a controlled unavailable error until the enterprise pool router exists.
- Notification outbox worker resolves and attaches typed tenant context before invoking the rule mapper or notification sender for organization-scoped events.
- Global notification events without `organization_id` continue through the existing global identity workflow.
- Resolution failure does not call the notification consumer and follows the existing outbox retry/dead transition.
- Unit tests cover identity, status, placement, context propagation, retry behavior, and global-event compatibility.
- PostgreSQL integration coverage verifies active, suspended, dedicated, and unknown organization behavior.

Scope:

- Load context from event organization ID.
- Validate organization status and data placement.
- Internal service identity rules.

Acceptance:

- Worker never processes tenant event without context.
- Unknown organization reaches controlled retry/dead state.

### MT-CORE-005: Require Tenant Middleware

Status: `done`

Progress:

- `RequireActiveTenant` fails closed when typed context is missing and rejects every non-active organization state.
- `RequireSetupTenant` explicitly allows only `pending`, `provisioning`, and `provisioning_failed` organization setup flows.
- `RequirePlatformTenant` and `RequireCustomerTenant` provide composable organization-type guards without treating platform operator roles as tenant identity.
- Context attachment exports organization, membership, resolution, placement, impersonation, request, authenticated user, and session fields for structured logging and audit consumers.
- Public module routes compose public host resolution with the active-tenant guard.
- Auth/User/Permission routes remain outside the generic tenant guard because they include global identity and platform operations.
- Organization and Landing admin handlers do not exist yet; their route groups must compose authenticated resolution with active/type/setup guards when introduced.
- Unit tests cover missing context, suspended organization, setup-only status, platform/customer separation, and structured fields.

Scope:

- Require active organization.
- Optional platform-only and customer-only guards.
- Organization status and setup-only behavior.
- Attach structured log/audit fields.

Acceptance:

- Tenant-owned admin route fails closed without context.
- Public and admin resolution use separate middleware chains.

### MT-CORE-006: Switch Organization

Status: done.

Implementation:

- Protected endpoints list active memberships and mark the organization stored in the current session.
- Membership responses include organization-scoped role IDs and slugs from `user_roles`; global platform roles are not mixed into tenant membership data.
- Switch locks the active session, validates an active membership and active organization, then updates the session organization/membership/version snapshot atomically.
- Successful switches write `organization_switched` audit metadata with the previous organization, session, membership, request, IP, and user agent context.
- Access and refresh tokens are not reissued because the current JWT claims do not contain organization identity; authenticated resolution reads the updated session snapshot on the next request.
- Unit, handler, and PostgreSQL integration coverage verify current markers, metadata propagation, session persistence, audit creation, and denied cross-organization switches.

Scope:

- `GET /users/me/organizations`
- `POST /users/me/switch-organization`
- Persist active organization in session.
- Rotate/reissue token if token contains organization claims.

Acceptance:

- Switch is audited.
- Previous stale context cannot continue after membership revocation.

### MT-CORE-007: Tenant-Scoped Permission Evaluation

Status: `done`

Implementation:

- Global permission evaluation only reads global role and direct permission assignments with `organization_id IS NULL`.
- Organization permission evaluation uses an explicit organization ID from verified tenant context and only reads assignments for that organization.
- Global and organization evaluation share role grants and direct allow/deny precedence without mixing scopes.
- `RequireOrganization` fails closed without authenticated user or verified tenant context and passes the trusted organization ID to the permission service.
- Auth login, refresh, and current-user permission payloads are restricted to global permissions because those flows do not carry verified organization context.
- Unit tests cover cross-organization role isolation, scoped deny behavior, and missing-context middleware rejection.
- PostgreSQL integration tests cover global/Organization A/Organization B isolation and auth permission payload filtering.

Scope:

- Separate global/platform permission evaluation from organization permission evaluation.
- Global evaluation only reads role and direct permission assignments where `organization_id IS NULL`.
- Organization evaluation requires verified tenant context and only reads assignments for the active organization.
- Direct deny overrides role and direct allow only inside the evaluated scope.
- Add explicit organization permission middleware for tenant-owned routes.

Acceptance:

- A permission assigned in Organization A cannot authorize a request in Organization B.
- Global permissions are not implicitly mixed into organization permission decisions.
- Organization permission checks fail closed when verified tenant context is missing.
- Existing global admin permission checks remain compatible.

## Phase 4 - Data Isolation

### MT-DATA-001: Tenant Transaction Helper

Status: `done`

Implementation:

- `TenantPoolResolver` selects a database transaction source from verified tenant placement.
- `SharedTenantPoolResolver` uses the application pool for `shared` placement and rejects `dedicated` placement with a stable error until the enterprise pool registry is implemented.
- `TenantTransactor.Begin` derives organization identity only from typed tenant context.
- Transactions set `app.organization_id` with transaction-local PostgreSQL configuration before repository work begins.
- `TenantTransactor.Within` owns commit/rollback, preserves callback errors, and uses an independent cleanup context when the request context is canceled.
- Unit tests cover missing context, missing callback, shared pool selection, and dedicated placement rejection.
- PostgreSQL integration tests prove the organization setting exists inside the transaction and is absent on the same pooled connection after commit and rollback.

Scope:

- Begin tenant transaction.
- Set local organization ID for RLS.
- Select shared/dedicated pool abstraction.
- Ensure cleanup on commit/rollback.

Acceptance:

- Pool connection cannot leak prior organization context.
- Missing organization cannot run tenant transaction.

### MT-DATA-002: Repository Scope Contract

Status: `done`

Implementation:

- Immutable `tenant.Scope` is derived only from verified tenant context and exposes organization identity plus data placement as read-only values.
- Single-row and bulk organization validators reject rows whose ownership differs from the active scope.
- Landing `PageRepository` requires scope on every operation and its create/update payloads do not accept organization identity.
- Landing cross-tenant reads use a separate `PlatformPageRepository` contract.
- Static AST tests inspect both tenant repository interfaces and concrete `*Repository` methods, requiring `coretenant.Scope` and rejecting free-form organization ID parameters.
- Unit tests cover missing context, valid scope derivation, valid batches, and mixed-organization rejection.

Scope:

- Establish interfaces/helpers requiring organization scope.
- Ban tenant `FindByID(id)` pattern.
- Bulk mixed-tenant validation.
- Explicit platform cross-tenant repository interface.

Acceptance:

- Code review/test tooling can identify unscoped methods.
- Create/update/delete derive organization from trusted context.

### MT-DATA-003: RLS Rollout

Scope:

- Apply policy to new Landing tables first.
- Expand to notification and other tenant tables after compatibility audit.

Acceptance:

- Repository filter and RLS both enforce isolation.
- Platform operator flow uses explicit controlled path.

### MT-DATA-004: Tenant Isolation Test Suite

Scope:

- Shared test fixtures for organization A/B.
- CRUD/read/list/bulk/export cross-tenant tests.
- RLS missing-context test.

Acceptance:

- Every tenant module can reuse the suite.
- Test fails when organization predicate is removed.

## Phase 5 - Platform and Organization API

### MT-API-001: Platform Organization Management

Scope:

- List/detail/create/update/status/archive organizations.
- Provision/retry provisioning.
- View placement, domain, entitlement, health summary.

Permission:

```txt
platform.organization.read
platform.organization.manage
platform.organization.suspend
platform.organization.provision
```

### MT-API-002: Organization Self Management

Scope:

- Current organization detail/settings.
- Update profile/timezone/locale.
- Member and invitation management.

Permission:

```txt
organization.read
organization.update
organization.member.read
organization.member.manage
```

### MT-API-003: Domain Management

Scope:

- Add/list/verify/set-primary/disable domain.
- Reserved subdomain checks.
- SSL state response.

### MT-API-004: Entitlement and Usage API

Scope:

- Organization effective features/limits/usage.
- Platform override management.

Acceptance:

- Customer cannot grant itself features.
- Override mutation is audited.

### MT-API-005: Operator Impersonation

Scope:

- Start/stop impersonation with reason and expiry.
- Expose effective/operator identity to frontend.

Acceptance:

- Explicit permission and audit required.
- Impersonation cannot become permanent session state.

## Phase 6 - Platform Organization and Landing

### MT-PLAT-001: Seed Platform Organization

Scope:

- Seed platform organization from config.
- Seed platform owner membership for super admin.
- Seed internal entitlement plan.
- Idempotent lookup by reserved slug/type.

Config example:

```env
PLATFORM_ORGANIZATION_SLUG=zyad-cloud
PLATFORM_ORGANIZATION_NAME=Zyad Cloud
PLATFORM_PRIMARY_DOMAIN=zyad.cloud
```

Acceptance:

- No duplicate platform organization.
- Platform organization cannot be removed by tenant API.

### MT-PLAT-002: Platform Domain Resolution

Scope:

- Register primary platform domain and optional `www` alias.
- Resolve to platform organization.
- Define redirect/canonical host behavior.

Acceptance:

- Platform marketing site does not rely on nullable ownership.
- Customer cannot claim reserved platform domain/subdomain.

### MT-PLAT-003: Landing Page Integration

Scope:

- Complete dependency for `LAND-0003`.
- Landing admin requires typed organization context, membership, permission, entitlement.
- Public Landing resolver uses host-resolved organization.
- Platform and customer landing data use identical repository scope contract.

Acceptance:

- Platform marketing page works through platform organization.
- Customer A cannot read/publish Customer B page.

## Phase 7 - Cache, Storage, Events, and Observability

### MT-INFRA-001: Tenant-Aware Cache

- Prefix/cache key contract.
- Membership/domain/entitlement version invalidation.
- Multi-instance invalidation.

### MT-INFRA-002: Tenant-Aware Storage

- Organization object prefix.
- Signed URL ownership check.
- Public published asset policy.

### MT-INFRA-003: Tenant Event Contract

- Standard event envelope with organization ID.
- Outbox publisher validation.
- Worker context loading.
- Organization-scoped idempotency.

### MT-INFRA-004: Audit and Structured Logging

- Add tenant, membership, resolution, operator/effective actor fields.
- Audit organization lifecycle, switch, domain, entitlement, impersonation.

### MT-INFRA-005: Tenant Metrics and Health

- Bounded global metrics.
- Platform organization and tenant health diagnostics.
- Avoid organization ID as uncontrolled metric label.

## Phase 8 - Dedicated Database Enterprise

Tasks in this phase are `deferred` until shared-schema tenancy is stable.

### MT-ENT-001: Data Placement Router

- Shared/dedicated placement.
- Pool registry and encrypted connection reference.

### MT-ENT-002: Dedicated Database Provisioning

- Create database/schema, credentials, migration, health check, rollback.

### MT-ENT-003: Fleet Migration

- Migration compatibility matrix.
- Batched rollout, retry, pause, and reporting.

### MT-ENT-004: Backup, Restore, and Export

- Per-tenant backup/restore.
- Shared-schema logical export.
- Region/data residency support.

### MT-ENT-005: Placement Migration

- Move shared to dedicated and reverse if supported.
- Dual-write or maintenance-window strategy.
- Reconciliation and cutover audit.

## Phase 9 - Security and Operations

### MT-SEC-001: Header and Host Security

- Header canonicalization.
- Trusted proxy.
- Host allowlist/canonicalization.
- Cache poisoning tests.

### MT-SEC-002: Cross-Tenant Security Tests

- IDOR.
- Forged header.
- Stale membership.
- Super admin without explicit platform permission.
- Bulk/export/storage/event isolation.

### MT-SEC-003: Organization Suspension and Incident Control

- Emergency suspend.
- Revoke sessions.
- Stop public serving/jobs.
- Security notification and audit.

### MT-OPS-001: Data Retention and Purge

- Archive, retention, legal hold, purge workflow.
- Database/storage/cache/provider cleanup.

### MT-OPS-002: Runbooks

- Provisioning failure.
- Domain conflict.
- Cross-tenant incident.
- Restore/export.
- Dedicated database outage.

## Phase 10 - Testing and Documentation

### MT-TEST-001: Unit Tests

- Context, host/header normalization, status, entitlement precedence, permission guards.

### MT-TEST-002: Repository Integration Tests

- Membership, domain, entitlement, active context, RLS.

### MT-TEST-003: End-to-End Tenant Flows

- Platform landing.
- Customer onboarding.
- User multi-membership/switch.
- Suspension.
- Cross-tenant denial.

### MT-DOC-001: API and Documentation Sync

- Update OpenAPI.
- Update Auth/User organization task references.
- Update Landing dependency/status.
- Add migration and operations documentation.

## Recommended Implementation Order

1. MT-0001..003
2. MT-DB-001..006
3. MT-BE-001..012
4. MT-CORE-001..006
5. MT-DATA-001..004
6. MT-PLAT-001..003
7. MT-API-001..004
8. MT-INFRA-001..004
9. MT-SEC-001..003
10. MT-ENT-* after shared-schema production validation

## Definition of Done

- Platform organization and customer organizations exist.
- Authenticated and public tenant resolution are separated.
- Membership is verified for every authenticated tenant request.
- Tenant repositories and RLS fail closed.
- Platform Landing Page resolves through platform organization.
- Permission, entitlement, audit, cache, storage, event, and jobs are tenant-aware.
- Cross-tenant tests pass.
- Migration, OpenAPI, runbook, and traceability are updated.
