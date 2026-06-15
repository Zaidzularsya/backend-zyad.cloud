# Multi-Tenant Schema and Query Audit

Audit date: 2026-06-13

Scope:

- Active migrations in `migrations/`.
- Repository and handler code that reads or writes `organization_id`.
- Legacy baseline ownership relevant to Landing Page.

This document is the output of `MT-0002`.

## Summary

The active schema does not yet contain `organizations` or `organization_memberships`.

Existing organization fields are references without foreign keys:

- `user_roles.organization_id`
- `user_permissions.organization_id`
- `notification_logs.organization_id`
- `notification_preferences.organization_id`
- `notification_outbox_events.organization_id`

The current code cannot enforce tenant isolation consistently because organization identity, membership, active context, and ownership foreign keys do not exist yet.

## Ownership Classification

### Platform Global

| Table | Classification | Notes |
| --- | --- | --- |
| `permissions` | platform global | Permission catalog |
| `roles` | platform global for MVP | System role catalog; custom organization roles need a later ownership decision |
| `role_permissions` | platform global | Role-to-permission policy |
| `notification_templates` | mixed/undecided | Current schema is global; tenant customization requires explicit organization ownership or override table |
| `schema_migrations` | platform global | Migration metadata |

### User Global

| Table | Classification | Notes |
| --- | --- | --- |
| `users` | user global | Identity shared across organizations |
| `user_profiles` | user global | Current profile is global |
| `auth_identities` | user global | Login identity |
| `password_reset_tokens` | user global | Credential recovery |
| `email_verification_tokens` | user global | Identity verification |
| `otp_codes` | user global | Auth/security purpose |
| `login_histories` | user global with optional tenant snapshot later | Login may occur before organization selection |
| `refresh_tokens` | user/session global | Inherits active organization through session |

### User Global with Organization Relation

| Table | Current State | Target |
| --- | --- | --- |
| `sessions` | migration `000017` adds active organization and membership/version snapshot | Resolver and switch flow pending |
| `user_roles` | nullable `organization_id`; unique `(user_id, role_id)` | Membership-aware assignment; allow same role in different organizations |
| `user_permissions` | nullable `organization_id` | Preserve global/platform override explicitly; validate organization FK |
| `notification_preferences` | nullable `organization_id` | Global user fallback plus organization override is intentional |

### Organization Scoped

| Table | Current State | Target |
| --- | --- | --- |
| `notification_logs` | nullable organization | Tenant business notifications should be non-null; platform-global security notifications need explicit classification |
| `notification_outbox_events` | nullable organization | Tenant business event should require organization; global event type must be explicit |
| Future Landing tables | not created | `organization_id NOT NULL` plus RLS |
| Future business module tables | mostly placeholders | Ownership classification required before implementation |

### Audit

| Table | Current State | Target |
| --- | --- | --- |
| `audit_logs` | no organization/effective actor | Add organization, membership, operator, impersonation metadata |

## Schema Findings

### MT-AUDIT-001: Organization Registry Migration Added

Severity: resolved at schema definition level

- Migration `000013_create_organizations` defines the organization registry.
- Existing organization UUID fields have no organization foreign key.
- `APP_ORGANIZATION_ID` exists but has no persisted organization contract.

Action:

- Apply migration `000013` in each environment before tenant-owned module migration.
- Seed a real platform organization rather than treating config ID as implicit ownership.

### MT-AUDIT-002: Membership Registry Migration Added

Severity: resolved at schema definition level

- Migration `000014_create_organization_memberships` defines membership lifecycle and versioning.
- Role assignment is currently the closest membership signal, but a member may exist before role assignment and membership has its own lifecycle.

Action:

- Apply migration `000014` after organizations exist.
- Migration backfills membership only for organization-scoped roles whose organization ID is valid.

### MT-AUDIT-003: User Role Uniqueness Normalized

Severity: resolved at schema definition level

Migration `000014` replaces the old constraint with:

```txt
UNIQUE (user_id, role_id) WHERE organization_id IS NULL
UNIQUE (user_id, role_id, organization_id) WHERE organization_id IS NOT NULL
```

Repository upsert behavior now preserves separate global and organization-scoped assignments. Effective permission queries must still be scoped by the verified active organization.

### MT-AUDIT-004: Organization Foreign Keys Are Missing

Severity: high

All current `organization_id` columns can reference nonexistent organizations.

Action:

- Backfill/clean invalid values.
- Add foreign keys after platform/customer organizations exist.
- Use `NOT VALID` followed by validation for safer rollout when needed.

### MT-AUDIT-005: Session Organization Context Migration Added

Severity: resolved at schema definition level

- Migration `000017_add_session_organization_context` adds active organization, membership, and membership version snapshot.
- JWT claims do not contain organization or membership version.
- Runtime resolver does not yet compare the snapshot with current membership version.

Action:

- Implement `MT-CORE-002` and `MT-CORE-006`.

### MT-AUDIT-006: Audit Log Cannot Reconstruct Cross-Tenant Action

Severity: resolved at schema definition level

Migration `000018_add_tenant_audit_metadata` adds:

- `organization_id`
- `membership_id`
- `session_id`
- operator/effective actor distinction
- impersonation session reference
- resolution source and request ID
- tenant/actor consistency validation and organization/time indexes

Action:

- Populate the context fields from tenant-aware audit writers during `MT-INFRA-004`.
- Keep platform cross-tenant APIs blocked until explicit permission and audit integration exist.

### MT-AUDIT-007: Notification Template Ownership Is Ambiguous

Severity: medium

- `notification_templates` has no `organization_id`.
- System templates are clearly global.
- Future tenant-custom templates need ownership without breaking system fallback.

Decision required during notification compatibility migration:

1. Add nullable `organization_id`, where null means system template only.
2. Or keep system templates global and add a tenant override table.

Preferred direction: tenant override table or explicit template scope to avoid overloaded null semantics.

### MT-AUDIT-008: Notification Organization Null Semantics Are Mixed

Severity: high

- Preferences intentionally allow global fallback.
- Logs and outbox allow null without explicit global event classification.

Action:

- Keep nullable organization for user-global preferences.
- Require organization for tenant business notification/event.
- Add explicit scope/event classification for legitimate platform-global notification.

### MT-AUDIT-009: Legacy Landing Ownership Uses Polymorphic Owner

Severity: high for Landing migration

Archived baseline uses:

```txt
owner_type
owner_id
```

with platform/account/resource ownership.

Target:

- Use `organization_id NOT NULL`.
- Represent Zyad Cloud through platform organization.
- Create explicit backfill mapping if legacy rows are present in the actual database.

## Query Findings

### MT-QUERY-001: Effective Permission Query Is Not Organization Scoped

Severity: critical

`permission.Repository.GetUserPermissions` reads all `user_roles` and `user_permissions` for a user without filtering active organization.

Risk:

- A role or permission from organization A may become effective in organization B.

Required fix:

- Effective permission API receives verified organization context.
- Include global platform assignment only through explicit policy.
- Evaluate direct deny/allow within the active organization.

### MT-QUERY-002: User List Role Projection Is Not Organization Scoped

Severity: high

The role aggregation in `UserRepository.ListUsers` selects all user roles without organization filtering.

Risk:

- Admin may see role names from another organization.

Required fix:

- Organization user list must start from membership.
- Role projection filters the active organization.
- Platform user directory is a separate explicit API.

### MT-QUERY-003: User List Organization Filter Is Membership-Incomplete

Severity: high

The existing organization filter uses `EXISTS` on `user_roles`.

Risk:

- A valid organization member without role is invisible.
- Membership lifecycle cannot be represented.

Required fix:

- Use `organization_memberships` as membership source.

### MT-QUERY-004: Notification Handlers Accept Client Organization

Severity: critical

Examples:

- Notification send request body contains `organization_id`.
- Preference and log handlers accept organization query parameters.

Risk:

- Forged organization selector can read/write another tenant if service/repository does not revalidate it.

Required fix:

- Self/admin tenant endpoints derive organization from verified context.
- Platform cross-tenant endpoint uses explicit platform permission and separate route/service method.

### MT-QUERY-005: Notification Log Filter Is Caller Scoped

Severity: critical

`NotificationLogRepository` applies organization filter only when caller supplies it.

Required fix:

- Organization log repository always requires organization.
- Cross-tenant platform log repository is explicit.

### MT-QUERY-006: Notification Preference Null Fallback Is Intentional

Severity: informational

The preference repository uses organization-specific rows with global fallback.

Action:

- Preserve behavior.
- Validate requested organization against active membership.
- Document global preference ownership as user-global.

### MT-QUERY-007: Outbox Worker Preserves Organization but Does Not Resolve It

Severity: high

The outbox event carries organization ID, but worker processing does not yet load and validate organization status/data placement.

Required fix:

- Implement `MT-CORE-004`.

## Config Findings

| Config | Current State | Action |
| --- | --- | --- |
| `APP_ORGANIZATION_ID` | Existing optional ID | Deprecate as implicit tenant; use only platform seed/backfill compatibility |
| `X-Org-Id` | Current middleware header | Temporary alias |
| `X-Tenant-ID` | README/CORS legacy header | Temporary alias |
| `X-Organization-ID` | New canonical header | Implement after authenticated resolver |
| Platform organization slug/name/domain | Implemented config contract | Consume during `MT-PLAT-001..002` |
| Trusted proxy CIDR | Implemented config and Gin policy | Consume forwarded host during `MT-CORE-003` |

## Migration Order Derived from Audit

1. Create organizations.
2. Create memberships and normalize organization role uniqueness.
3. Create organization domain and reserved subdomain registry.
4. Create entitlement and usage registry.
5. Seed platform organization, owner, primary domain, reserved labels, and internal entitlement.
6. Validate/backfill existing organization IDs.
7. Add session active organization/membership version.
8. Add audit organization/effective actor fields.
9. Add remaining organization foreign keys.
10. Introduce typed resolver and switch API.
11. Fix permission/user/notification queries.
12. Enable first RLS policies on new Landing tables.

## Implementation Blockers

Do not implement tenant-owned Landing repository/API before:

- `MT-DB-001`
- `MT-DB-002`
- `MT-CORE-001`
- `MT-CORE-002`
- `MT-DATA-001`

Domain models and DTOs that do not access persistence may continue.

## Audit Completion Criteria

- Active tables are classified. `done`
- Existing organization fields are inventoried. `done`
- High-risk queries are identified. `done`
- Legacy Landing ownership conflict is identified. `done`
- Migration order is documented. `done`
