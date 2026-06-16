# Multi-Tenant PostgreSQL RLS

## Runtime Contract

Tenant-owned repository operations must run through `TenantTransactor`. The
transaction sets the verified organization identity with:

```sql
SELECT set_config('app.organization_id', '<organization-uuid>', true);
```

The setting is transaction-local. Missing or empty settings make
`app_current_organization_id()` return `NULL`, so tenant policies deny reads and
writes.

New tenant-owned tables must:

- define `organization_id uuid NOT NULL`;
- include `organization_id` in tenant-scoped unique constraints;
- keep repository organization predicates as the primary isolation guard; and
- call `apply_organization_rls('<table_name>'::regclass)` in their migration.

The helper installs:

- a restrictive organization boundary policy; and
- a permissive tenant access policy required by PostgreSQL policy composition.

Use `remove_organization_rls('<table_name>'::regclass)` before dropping a
tenant-owned table in its down migration.

## Database Roles

The application runtime role must be a non-superuser role without
`BYPASSRLS`. It must not own tenant-owned tables.

Migration and controlled maintenance use a separate role that owns schema
objects. Cross-tenant platform operations must use an explicit audited
maintenance path; application headers, request parameters, and ordinary
platform permissions never disable RLS.

Example role checks:

```sql
SELECT rolname, rolsuper, rolbypassrls
FROM pg_roles
WHERE rolname IN ('zyad_runtime', 'zyad_migration');
```

Expected runtime values are `rolsuper = false` and `rolbypassrls = false`.

## Rollout

Migration `000019_add_rls_foundation` provides the shared helpers. The first
production policies are applied by the Landing table migrations because those
tables use mandatory organization ownership from their first release.

Notification tables remain excluded until nullable organization semantics and
platform-global events are normalized. Organization control-plane tables also
remain excluded until their public resolver and platform maintenance paths use
explicit RLS-compatible access.
