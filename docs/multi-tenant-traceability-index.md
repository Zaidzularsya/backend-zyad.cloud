# Multi-Tenant Traceability Index

Status:

- `planned`
- `in_progress`
- `done`
- `deferred`
- `blocked`

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-multi-tenant.md` | Architecture and capability reference |
| `docs/multi-tenant-development-tasks.md` | Development breakdown |
| `docs/reference-auth-user.md` | User multi-organization requirement |
| `docs/reference-landing-page.md` | Tenant-owned and platform-owned Landing Page |
| `docs/migration-guide.md` | Migration rules |
| `README.md` | Platform multi-tenant objective |
| `docs/multi-tenant-schema-query-audit.md` | Active schema and query risk audit |

## Existing State

| Existing Capability | Location | Status/Gap |
| --- | --- | --- |
| Typed tenant context | `internal/core/tenant`, `internal/core/middleware/tenant.go` | foundation implemented; resolver pending |
| Organization-scoped user roles | `user_roles.organization_id` | separate global/organization uniqueness in migration `000014`; resolver scoping pending |
| Organization-scoped direct permission | `user_permissions.organization_id` | exists |
| Notification organization fields | notification tables | partial |
| Current organization API contract | Auth/User docs | planned |
| Organization module | `internal/modules/organization` | models, DTOs, and organization repository implemented |
| Multi-tenant config contract | `internal/config`, `.env.example` | platform identity/domain, placement, and trusted proxy validation implemented |
| Platform organization | `organizations` schema in migration `000013`, `cmd/seed -name platform-organization` | schema and idempotent config seed done |
| Organization membership table | `organization_memberships` in migration `000014` | repository, lifecycle service, role synchronization, and session invalidation ready |
| Organization domain registry | `organization_domains`, `reserved_subdomains` in migration `000015` | repository and public HTTP resolver ready; management API pending |
| Entitlement and usage registry | `organization_entitlements`, `organization_usage_counters` in migration `000016` | repository and evaluation/quota service ready |
| Session organization context | `sessions` columns and `organization_impersonation_sessions` in migration `000017` | authenticated resolver and switch ready; impersonation API pending |
| Tenant audit metadata | `audit_logs` columns and validation in migration `000018` | schema and organization lifecycle/membership/switch audit writers ready |
| RLS | Migration `000019` helpers and role contract | foundation done; Landing core page/section tables adopted RLS in `000025`; repository rollout pending |
| Dedicated database routing | None | deferred target |

## Requirement Traceability

| Requirement | Task | Data/API | Status |
| --- | --- | --- | --- |
| Canonical organization terminology | MT-0001 | Context contract | done |
| Platform organization | MT-DB-001, MT-PLAT-001, MT-PLAT-002 | `organizations`, `organization_domains` | schema, organization seed, and platform domain seed done |
| Customer organization lifecycle | MT-BE-010 | Organization APIs | transaction/status, platform management API, and generic tenant event contract done; concrete event producers pending |
| User multi-membership | MT-DB-002, MT-BE-011 | `organization_memberships`, membership service | backend service and self organization context API done; admin API planned |
| Active organization switching | MT-CORE-006 | `/users/me/organizations`, `/users/me/switch-organization` | done |
| Authenticated tenant resolution | MT-CORE-002 | Session/header/single membership | done |
| Public host resolution | MT-CORE-003 | Platform host/domain registry/trusted proxy | done |
| Worker tenant context | MT-CORE-004 | Notification outbox organization ID/internal identity | done |
| Fail-closed middleware | MT-CORE-005 | Active/setup/platform/customer guards and public module chain | done |
| Shared-schema isolation | MT-DATA-001..004 | Repository scope and RLS foundation done; Landing core table policy rollout started in `000025`; repository isolation suite adoption done |
| Dedicated database tenancy | MT-ENT-001..005 | Placement router | deferred |
| Organization domain | MT-DB-003, MT-API-003 | `organization_domains`, `reserved_subdomains` | schema, verification repository, and management API done |
| Feature entitlement | MT-DB-004, MT-BE-012, MT-API-004 | `organization_entitlements` | schema, service, effective feature API, and platform override API done |
| Quota/usage | MT-DB-004, MT-BE-012, MT-API-004 | `organization_usage_counters` | schema, service, and usage read API done |
| Platform/customer authorization separation | MT-CORE-005, MT-API-001 | Tenant type guards done; platform management permission/API planned |
| Operator impersonation | MT-DB-005, MT-API-005 | `organization_impersonation_sessions` | schema, start/stop service, API, permission, and audit done |
| Tenant-aware cache | MT-INFRA-001 | Redis/cache keys | done |
| Tenant-aware storage | MT-INFRA-002 | Object prefix | done |
| Tenant event envelope | MT-INFRA-003 | Outbox/jobs | done |
| Tenant audit/logging | MT-DB-006, MT-INFRA-004 | Audit/log fields | schema, organization audit writers, and structured logger helpers done; audit read API planned |
| Tenant metrics/health | MT-INFRA-005 | Bounded metrics and health diagnostics | done |
| Platform marketing Landing Page | MT-PLAT-002, MT-PLAT-003 | Platform host/page | platform host/domain foundation done; Landing integration planned |
| Customer Landing isolation | MT-DATA-001..004, MT-PLAT-003 | Landing page/section RLS tables and access policy done; concrete Landing repositories pending |
| Header and host security | MT-SEC-001 | Tenant selector and public host middleware | done |
| Suspension/incident control | MT-SEC-003 | Lifecycle/session/jobs/security event | done |
| Retention/deletion | MT-OPS-001 | Purge workflow | planned |
| Cross-tenant security | MT-SEC-002 | Security tests | done |

## Task Index

| Phase | Task ID | Status |
| --- | --- | --- |
| Contract/audit | MT-0001 | done |
| Contract/audit | MT-0002 | done |
| Contract/audit | MT-0003 | done |
| Database | MT-DB-001 | done |
| Database | MT-DB-002 | done |
| Database | MT-DB-003 | done |
| Database | MT-DB-004 | done |
| Database | MT-DB-005 | done |
| Database | MT-DB-006 | done |
| Database | MT-DB-007 | done |
| Domain/DTO | MT-BE-001 | done |
| Domain/DTO | MT-BE-002 | done |
| Repository | MT-REPO-001 | done |
| Repository | MT-REPO-002 | done |
| Repository | MT-REPO-003 | done |
| Repository | MT-REPO-004 | done |
| Service | MT-BE-010 | done |
| Service | MT-BE-011 | done |
| Service | MT-BE-012 | done |
| Context/resolution | MT-CORE-001 | done |
| Context/resolution | MT-CORE-002 | done |
| Context/resolution | MT-CORE-003 | done |
| Context/resolution | MT-CORE-004 | done |
| Context/resolution | MT-CORE-005 | done |
| Context/resolution | MT-CORE-006 | done |
| Authorization | MT-CORE-007 | done |
| Data isolation | MT-DATA-001 | done |
| Data isolation | MT-DATA-002 | done |
| Data isolation | MT-DATA-003 | done |
| Data isolation | MT-DATA-004 | done |
| API | MT-API-001 | done |
| API | MT-API-002 | done |
| API | MT-API-003 | done |
| API | MT-API-004 | done |
| API | MT-API-005 | done |
| Platform/Landing | MT-PLAT-001 | done |
| Platform/Landing | MT-PLAT-002 | done |
| Platform/Landing | MT-PLAT-003 | done |
| Infrastructure | MT-INFRA-001 | done |
| Infrastructure | MT-INFRA-002 | done |
| Infrastructure | MT-INFRA-003 | done |
| Infrastructure | MT-INFRA-004 | done |
| Infrastructure | MT-INFRA-005 | done |
| Enterprise dedicated DB | MT-ENT-001 | deferred |
| Enterprise dedicated DB | MT-ENT-002 | deferred |
| Enterprise dedicated DB | MT-ENT-003 | deferred |
| Enterprise dedicated DB | MT-ENT-004 | deferred |
| Enterprise dedicated DB | MT-ENT-005 | deferred |
| Security | MT-SEC-001 | done |
| Security | MT-SEC-002 | done |
| Security | MT-SEC-003 | done |
| Operations | MT-OPS-001 | planned |
| Operations | MT-OPS-002 | planned |
| Test/docs | MT-TEST-001 | planned |
| Test/docs | MT-TEST-002 | planned |
| Test/docs | MT-TEST-003 | planned |
| Test/docs | MT-DOC-001 | planned |

## API Traceability

Endpoint naming is a target contract and must be synchronized to OpenAPI during implementation.

| Endpoint | Task | Permission | Status |
| --- | --- | --- | --- |
| `GET /platform/organizations` | MT-API-001 | `platform.organization.read` | done |
| `POST /platform/organizations` | MT-API-001 | `platform.organization.manage` | done |
| `GET /platform/organizations/:id` | MT-API-001 | `platform.organization.read` | done |
| `PATCH /platform/organizations/:id` | MT-API-001 | `platform.organization.manage` | done |
| `PATCH /platform/organizations/:id/status` | MT-API-001 | `platform.organization.suspend` | done |
| `POST /platform/organizations/:id/provision` | MT-API-001 | `platform.organization.provision` | done for shared placement; dedicated returns controlled unavailable |
| `POST /platform/organizations/:id/impersonate` | MT-API-005 | `platform.organization.impersonate` | done |
| `DELETE /platform/impersonation` | MT-API-005 | `platform.organization.impersonate` | done |
| `GET /users/me/organizations` | MT-CORE-006 | authenticated user | done |
| `POST /users/me/switch-organization` | MT-CORE-006 | active membership | done |
| `GET /organization` | MT-API-002 | `organization.read` | done |
| `PATCH /organization` | MT-API-002 | `organization.update` | done |
| `GET /organization/members` | MT-API-002 | `organization.member.read` | done |
| `POST /organization/invitations` | MT-API-002 | `organization.member.manage` | done |
| `PATCH /organization/members/:id/status` | MT-API-002 | `organization.member.manage` | done |
| `DELETE /organization/members/:id` | MT-API-002 | `organization.member.manage` | done |
| `GET /organization/domains` | MT-API-003 | `organization.domain.manage` | done |
| `POST /organization/domains` | MT-API-003 | `organization.domain.manage` | done |
| `POST /organization/domains/:id/verify` | MT-API-003 | `organization.domain.manage` | done |
| `PATCH /organization/domains/:id` | MT-API-003 | `organization.domain.manage` | done |
| `DELETE /organization/domains/:id` | MT-API-003 | `organization.domain.manage` | done |
| `GET /organization/features` | MT-API-004 | `organization.feature.read` | done |
| `GET /organization/usage` | MT-API-004 | `organization.feature.read` | done |
| `PATCH /platform/organizations/:id/entitlements` | MT-API-004 | `platform.organization.manage` | done |

## Migration Traceability

Final migration number is assigned during implementation.

| Logical Migration | Task | Status |
| --- | --- | --- |
| `000013_create_organizations` | MT-DB-001 | done |
| `000014_create_organization_memberships` | MT-DB-002 | done |
| `000015_create_organization_domains` | MT-DB-003 | done |
| `000016_create_organization_entitlements` | MT-DB-004 | done |
| `000017_add_session_organization_context` | MT-DB-005 | done |
| `000018_add_tenant_audit_metadata` | MT-DB-006 | done |
| `000019_add_rls_foundation` | MT-DB-007, MT-DATA-003 | foundation done; Landing policy adoption pending |
| Platform organization/owner/internal entitlement seed command | MT-PLAT-001 | done |
| Platform primary and www domain registration | MT-PLAT-002 | done |
| `000020_seed_platform_organization_permissions` | MT-API-001 | done |
| `000021_seed_organization_self_permissions` | MT-API-002 | done |
| `000022_seed_organization_domain_permission` | MT-API-003 | done |
| `000023_seed_organization_feature_permission` | MT-API-004 | done |
| `000024_seed_platform_impersonation_permission` | MT-API-005 | done |

## Permission Traceability

| Permission | Capability | Status |
| --- | --- | --- |
| `platform.organization.read` | Cross-tenant organization read | done |
| `platform.organization.manage` | Create/update/archive/entitlement | done |
| `platform.organization.suspend` | Suspend/restore tenant | done |
| `platform.organization.impersonate` | Start/stop impersonation | done |
| `platform.organization.provision` | Provision/retry placement | done for shared placement; dedicated deferred |
| `platform.domain.manage` | Platform/reserved domain | planned |
| `platform.audit.read` | Cross-tenant audit read | planned |
| `organization.read` | Current organization detail | done |
| `organization.update` | Current organization settings | done |
| `organization.member.read` | Member list | done |
| `organization.member.manage` | Invitation/member status | done |
| `organization.role.manage` | Organization roles | planned |
| `organization.domain.manage` | Tenant domains | done |
| `organization.audit.read` | Tenant audit | planned |
| `organization.billing.read` | Tenant billing summary | planned |
| `organization.feature.read` | Feature/usage read | done |

## Landing Page Dependency

| Landing Requirement | Multi-Tenant Dependency | Status |
| --- | --- | --- |
| `LAND-0003` typed organization context | MT-CORE-001..005 | typed context, authenticated/public resolvers, and fail-closed guards done |
| Platform marketing Landing Page | MT-PLAT-001..003 | platform organization, platform domain, and Landing scope contract done |
| Customer page isolation | MT-DATA-001..004, MT-PLAT-003 | repository scope, Landing access policy, and page/section RLS tables done; concrete Landing repositories pending |
| Public custom domain resolution | MT-CORE-003, MT-API-003 | runtime resolver and domain management API done |
| Landing feature/limit | MT-BE-012, MT-API-004 | done |
| Tenant media/storage | MT-INFRA-002 | object prefix and ownership contract done; Landing media metadata pending |
| Tenant event/notification | MT-INFRA-003 | envelope and notification publisher adapter done; concrete producers pending |

## Auth/User Dependency

| Auth/User Requirement | Multi-Tenant Task | Status |
| --- | --- | --- |
| USER-0401 multi organization | MT-DB-001..002, MT-BE-010..011, MT-CORE-006 | self list/switch and context integration done; organization admin users API remains planned |
| Current organization bootstrap | MT-CORE-001..002, MT-CORE-006 | typed context, authenticated resolution, and session switch persistence done |
| Switch organization | MT-CORE-006 | done |
| Organization-scoped role | MT-BE-011, MT-CORE-006..007 | membership role synchronization, role read model, and tenant-scoped permission evaluator done; tenant route adoption follows each API implementation |
| Session invalidation | MT-DB-005, MT-BE-011 | schema, mutation revocation, and stale snapshot resolver enforcement done |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-13 | Organization is the canonical business tenant | Existing role/permission and docs already use organization scope |
| 2026-06-13 | Zyad Cloud uses a platform organization | Platform Landing Page can use the same tenant-safe module path |
| 2026-06-13 | Shared database/shared schema is MVP | Lower operational complexity; dedicated database remains enterprise target |
| 2026-06-13 | Canonical selector is `X-Organization-ID` | Aligns API with business terminology; old headers become compatibility aliases |
| 2026-06-13 | Raw organization header is never trusted | Membership and organization status must be verified |
| 2026-06-13 | Platform operator access is explicit and audited | A role name alone must not silently bypass tenant isolation |
| 2026-06-13 | Repository filter plus PostgreSQL RLS | Layered defense against cross-tenant data leakage |
| 2026-06-13 | Organization owns tenant identity; Landing owns page mapping | Avoid duplicate business ownership while supporting custom domains |
| 2026-06-14 | Entitlement source precedence is override, addon, trial, then plan | Platform overrides must win explicitly, paid additions extend plans, and temporary trials remain above the base plan |
| 2026-06-15 | Notification outbox is not reused for organization provisioning events | A generic tenant business-event envelope belongs to MT-INFRA-003 and must not be coupled to notification delivery semantics |
| 2026-06-15 | Global and organization permission evaluation use separate explicit paths | Prevent organization assignments from authorizing global or other-organization requests |
| 2026-06-15 | Tenant database context uses transaction-local PostgreSQL settings | `SET LOCAL` is reset automatically on commit/rollback and cannot leak through the connection pool |
| 2026-06-15 | Tenant repositories receive immutable scope; platform repositories are separate | Prevent tenant repositories from accepting caller-controlled organization identity while preserving explicit cross-tenant administration |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-13 | Initial multi-tenant architecture documentation | Reference, tasks, traceability | Includes platform organization and Landing Page dependency |
| 2026-06-13 | Completed terminology/context contract | `internal/core/tenant`, tenant middleware, organization module | Added typed verified context, canonical/legacy header contract, and initial organization models |
| 2026-06-13 | Completed active schema/query audit | `docs/multi-tenant-schema-query-audit.md` | Identified membership, permission, notification, session, audit, and legacy Landing isolation gaps |
| 2026-06-13 | Completed config and trusted proxy contract | `internal/config`, `.env.example`, `internal/app/router.go` | Added production platform config validation and disabled Gin's implicit trust-all proxy default |
| 2026-06-13 | Added organizations database foundation | `migrations/000013_create_organizations.*.sql` | Added constrained organization registry, normalized active slug uniqueness, and platform removal protection |
| 2026-06-13 | Added organization membership foundation | `migrations/000014_create_organization_memberships.*.sql` | Added membership lifecycle/versioning, valid-role backfill, organization role uniqueness, and role assignment compatibility |
| 2026-06-13 | Added organization domain foundation | `migrations/000015_create_organization_domains.*.sql` | Added canonical host, verification and SSL lifecycle, primary host policy, and reserved subdomain conflict guards |
| 2026-06-13 | Added entitlement and usage foundation | `migrations/000016_create_organization_entitlements.*.sql` | Added time-windowed entitlement sources, auditable overrides, versioning, and period-based usage counters |
| 2026-06-13 | Added active session context and impersonation foundation | `migrations/000017_add_session_organization_context.*.sql` | Added membership-version snapshot validation and retained operator/target impersonation metadata |
| 2026-06-14 | Added tenant audit metadata | `migrations/000018_add_tenant_audit_metadata.*.sql`, user audit model/repository/DTO | Added organization, membership, session, operator/effective actor, impersonation, resolution, and request metadata |
| 2026-06-14 | Completed organization models and DTO contracts | `internal/core/tenant`, `internal/modules/organization/model`, `internal/modules/organization/dto` | Aligned enums with migrations and added lifecycle, membership, domain, entitlement, usage, and switch contracts without tenant mass assignment |
| 2026-06-14 | Applied tenant audit migration | database `platform`, migration `000018_add_tenant_audit_metadata` | Confirmed migrations `000013` through `000018` and all tenant audit columns in `public.audit_logs` |
| 2026-06-14 | Completed organization repository | `internal/modules/organization/repository` | Added canonical slug CRUD, lifecycle status/archive operations, platform lookup, default archived filtering, and PostgreSQL integration coverage |
| 2026-06-14 | Completed membership repository | `internal/modules/organization/repository` | Added membership lifecycle/version lookup, user and organization lists, ownership transfer, and organization-serialized last-owner protection with concurrent integration coverage |
| 2026-06-14 | Completed organization domain repository | `internal/modules/organization/repository` | Added challenge, verification, primary, SSL, reassignment, reserved-label protection, and active verified host resolution with PostgreSQL integration coverage |
| 2026-06-14 | Completed entitlement repository | `internal/modules/organization/repository` | Added deterministic effective source lookup, advisory-locked upsert, cache version metadata, structured limits, and atomic period usage limits |
| 2026-06-15 | Implemented organization lifecycle transaction and status rules | `internal/modules/organization/service`, `internal/modules/organization/repository` | Added atomic organization/owner/plan creation, rollback coverage, lifecycle audit, stale-state guard, platform protection, and session revocation; generic provisioning event remains pending |
| 2026-06-15 | Completed entitlement evaluation and quota service | `internal/modules/organization/service/entitlement_service.go`, `internal/modules/organization/service/entitlement_service_test.go` | Added fail-closed effective feature checks, structured hard-limit validation, explicit-period usage checks, atomic quota consumption, and stable error mapping |
| 2026-06-15 | Completed typed tenant context and authenticated resolver | `internal/core/tenant`, `internal/core/middleware/tenant.go`, `internal/modules/organization/repository/authenticated_resolver_repository.go`, `internal/modules/organization/service/authenticated_resolver.go`, `internal/app` | Added explicit missing-context errors, membership-version context validation, verified session/header/single-membership resolution, stale snapshot rejection, and protected-route wiring |
| 2026-06-15 | Completed public host resolver | `internal/core/middleware/public_tenant.go`, `internal/modules/organization/repository/public_host_resolver_repository.go`, `internal/modules/organization/service/public_host_resolver.go`, `internal/app` | Added strict host normalization, platform/custom/subdomain resolution, trusted forwarded-host handling, public selector rejection, active organization enforcement, and public module route wiring |
| 2026-06-15 | Completed worker/internal tenant resolver | `internal/modules/organization/service/worker_resolver.go`, `internal/core/notification/consumer/outbox_worker.go`, `cmd/worker/main.go` | Added allowlisted service identity, active organization/shared-placement validation, typed worker context propagation, and controlled notification outbox retry/dead behavior |
| 2026-06-15 | Completed tenant requirement middleware | `internal/core/middleware/tenant.go`, `internal/core/middleware/tenant_test.go`, `internal/app/router.go` | Added active/setup/platform/customer guards, structured tenant log fields, fail-closed coverage, and active public module route enforcement |
| 2026-06-15 | Completed tenant-scoped permission evaluation | `internal/core/permission`, `internal/modules/user/repository/auth_repository.go` | Added separate global/organization permission queries, fail-closed organization middleware, auth permission scope filtering, and cross-tenant unit/integration coverage |
| 2026-06-15 | Completed tenant transaction helper | `internal/platform/database/tenant_transaction.go` | Added verified-context pool resolution, transaction-local organization setting, dedicated placement rejection, callback transaction lifecycle, and same-connection cleanup tests |
| 2026-06-15 | Completed tenant repository scope contract | `internal/core/tenant/scope.go`, `internal/modules/landing/repository` | Added immutable verified scope, single/bulk ownership validation, separate platform repository boundary, and static unscoped-method detection |
| 2026-06-16 | Completed tenant-aware cache contract | `internal/core/cache`, `internal/platform/redis` | Added tenant-scoped key builder, versioned domain/permission/entitlement/Landing keys, and Redis invalidation event payload/channel contract |
| 2026-06-16 | Completed tenant-aware storage contract | `internal/platform/storage` | Added organization-prefixed object keys, signed object ownership validation, and published public asset URL policy |
| 2026-06-16 | Completed tenant event contract | `internal/core/event`, `internal/core/notification/publisher` | Added scope-derived tenant event envelope, organization-scoped idempotency key, worker context helper, and notification outbox adapter |
| 2026-06-16 | Completed tenant audit and structured logging contract | `migrations/000018_add_tenant_audit_metadata.*.sql`, `internal/modules/organization/repository`, `internal/platform/logger` | Added domain audit coverage plus reusable tenant slog attributes for organization, membership, actor, resolution, request, and impersonation context |
| 2026-06-16 | Completed tenant metrics and health contract | `internal/platform/metrics`, `internal/modules/organization/service` | Added bounded metric label validation, high-cardinality label rejection, and deterministic platform health issue coverage |
| 2026-06-16 | Completed header and host security hardening | `internal/core/middleware/public_tenant.go`, `internal/modules/organization/service/public_host_resolver.go` | Added trusted forwarded-host validation, public selector rejection coverage, and cache-poisoning host normalization tests |
| 2026-06-16 | Completed cross-tenant security test coverage | `internal/modules/organization/handler`, `internal/platform/storage`, `internal/core/event` | Added explicit platform permission denial coverage, storage asset metadata isolation, event payload tenant override protection, and documented existing forged-header, stale-membership, IDOR, bulk/export isolation coverage |
| 2026-06-16 | Completed organization suspension and incident control | `internal/modules/organization/service`, `internal/app` | Added best-effort platform security notification event for incident statuses and documented existing transactional session revocation, audit, public serving, and worker stop controls |
| 2026-06-17 | Started Landing production RLS rollout | `migrations/000025_create_landing_page_core_tables.*.sql` | Landing pages and sections now have `organization_id NOT NULL`, tenant uniqueness, and forced RLS; repository isolation adoption remains under Landing repository tasks |
| 2026-06-18 | Completed repository tenant isolation tests | `testutil/tenant_isolation.go`, `internal/modules/landing/repository/*` | Validated tenant scope boundary for Page, Section, Form, Submission, and Media repositories via `RunTenantIsolationSuite`. MT-DATA-004 is done. |

## Update Rules

- Mark task `in_progress` when implementation starts.
- Record actual migration filenames when created.
- Update Auth/User and Landing traceability when dependency completes.
- Update OpenAPI for endpoint changes.
- Mark `done` only after isolation tests pass.
- Record any cross-tenant architecture decision in Decision Log.
