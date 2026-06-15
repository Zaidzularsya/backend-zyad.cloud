# Multi-Tenant Platform Reference

Dokumen ini menetapkan requirement dan boundary multi-tenant untuk Zyad Cloud.

## Sources

- `README.md`
- `docs/reference-auth-user.md`
- `docs/auth-user-development-tasks.md`
- `docs/auth-user-migration-seed-plan.md`
- `docs/reference-landing-page.md`
- Implementasi awal `internal/core/middleware/tenant.go`
- Role dan permission organization-scoped yang sudah tersedia
- Baseline schema arsip di `docs/archive/migrations/monolithic_baseline`

## Objective

Membangun platform yang:

- Melayani Zyad Cloud sebagai operator platform.
- Melayani banyak organization pelanggan sebagai tenant.
- Menyediakan Landing Page marketing untuk platform Zyad Cloud sendiri.
- Menyediakan Landing Page dan module produk untuk tenant pelanggan.
- Menjamin data isolation, access control, dan audit lintas tenant.
- Mendukung shared database/shared schema sebagai default.
- Menyiapkan dedicated database tenant enterprise tanpa memaksakan kompleksitas tersebut pada MVP.

## Terminology

| Term | Meaning |
| --- | --- |
| Platform | Sistem dan operator Zyad Cloud |
| Platform organization | Tenant khusus yang memiliki data bisnis Zyad Cloud sendiri |
| Customer organization | Tenant pelanggan yang berlangganan Zyad Cloud |
| Organization | Istilah canonical untuk tenant bisnis |
| Membership | Relasi user dengan organization |
| Active organization | Organization yang sedang dipakai pada request authenticated |
| Control plane | Organization registry, provisioning, plans, routing, dan operator management |
| Data plane | Data module bisnis milik suatu organization |
| Platform-global data | Data teknis yang sengaja tidak dimiliki organization, misalnya migration metadata |

Gunakan `organization_id` pada model, database, context, dan API. Istilah `tenant_id` hanya boleh dipakai pada abstraction infrastructure atau compatibility layer, bukan sebagai column bisnis kedua.

## Platform Organization

Zyad Cloud sendiri harus direpresentasikan sebagai organization:

```txt
type = platform
slug = zyad-cloud
```

Platform organization memiliki:

- Landing Page marketing Zyad Cloud.
- Branding platform.
- Lead dari calon pelanggan platform.
- User internal Zyad Cloud.
- Notification dan audit milik operasi platform.

Customer organization memakai type:

```txt
type = customer
```

Keuntungan model ini:

- Landing Page platform memakai module yang sama dengan tenant.
- Tidak membutuhkan ownership nullable atau branch khusus di repository Landing.
- Permission, audit, storage, notification, dan analytics tetap konsisten.
- Platform content terisolasi dari customer content.

Platform organization bukan pengganti operator privilege. Aksi lintas tenant tetap membutuhkan platform role/permission dan audit impersonation.

## Ownership Classification

Setiap table harus diklasifikasikan:

### Platform Global

Tidak memiliki `organization_id`:

- `schema_migrations`
- organization registry
- organization database placement/routing metadata
- platform plan catalog
- module catalog
- trusted domain routing registry jika menjadi control-plane table

### Organization Scoped

Wajib memiliki `organization_id NOT NULL`:

- Landing Page, forms, submissions, branding, media, analytics.
- Product/configuration tenant.
- Notification log/preference yang merupakan aktivitas tenant.
- Billing/customer operational data.
- Role assignment dan direct permission dengan scope organization.

### User Global with Organization Relation

- User identity dan credential tetap global.
- Membership, role, permission, preference, dan current context menghubungkan user ke organization.
- Email yang sama dapat menjadi anggota beberapa organization.

Tidak boleh menggunakan `organization_id NULL` untuk berarti platform organization. Gunakan ID platform organization yang nyata.

## Isolation Strategy

### Default: Shared Database, Shared Schema

- Semua tenant-scoped row memiliki `organization_id NOT NULL`.
- Unique constraint tenant-scoped selalu memasukkan `organization_id`.
- Repository method tenant-scoped wajib menerima `TenantContext` atau `organizationID`.
- Query tenant tidak boleh hanya mengandalkan filter yang diberikan client.
- Transaction helper dapat mengatur PostgreSQL session variable untuk RLS.
- PostgreSQL Row-Level Security menjadi defense-in-depth, bukan satu-satunya guard.

### Enterprise: Dedicated Database

Disiapkan sebagai target:

- Organization memiliki `data_placement = dedicated`.
- Control plane menyimpan connection reference terenkripsi, bukan password plain.
- Resolver memilih pool berdasarkan organization setelah identity tervalidasi.
- Semua tenant database memakai migration version yang kompatibel.
- Provisioning, migration rollout, backup, restore, health, dan deprovisioning harus terotomasi.

MVP tetap memakai shared database. Dedicated database tidak boleh diimplementasikan setengah jadi melalui connection string dari request.

## Tenant Context

Canonical context:

```txt
TenantContext
- organization_id
- organization_slug
- organization_type
- organization_status
- membership_id
- membership_role/status
- resolution_source
- data_placement
- request_host
- is_platform_operator
- impersonation_session_id optional
```

Context harus tersedia pada:

- `gin.Context`
- `context.Context` request untuk service/repository
- audit log
- log fields dan tracing
- notification/outbox event

Context immutable setelah resolution. Handler tidak boleh mengganti organization secara ad hoc.

## Resolution Modes

## Configuration Contract

| Environment Variable | Purpose |
| --- | --- |
| `PLATFORM_ORGANIZATION_ID` | Stable UUID used to seed and find the platform organization |
| `PLATFORM_ORGANIZATION_SLUG` | Reserved platform organization slug |
| `PLATFORM_ORGANIZATION_NAME` | Platform organization display name |
| `PLATFORM_PRIMARY_DOMAIN` | Canonical platform marketing domain without scheme, port, or path |
| `PLATFORM_RESERVED_SUBDOMAINS` | Comma-separated labels unavailable to customer organizations |
| `TENANT_DEFAULT_DATA_PLACEMENT` | Default `shared` or `dedicated` placement |
| `TRUSTED_PROXY_CIDRS` | Comma-separated reverse proxy CIDRs trusted by the HTTP server |
| `TRUST_FORWARDED_HOST` | Allows future host resolver to use forwarded host only through a trusted proxy |

`APP_ORGANIZATION_ID` is a deprecated compatibility fallback for
`PLATFORM_ORGANIZATION_ID`. New deployments must use the platform-prefixed
variable.

Production startup requires platform organization ID, slug, name, primary
domain, and valid authentication secrets. `TRUST_FORWARDED_HOST=true` is
invalid when `TRUSTED_PROXY_CIDRS` is empty. The Gin router trusts no proxy by
default when the CIDR list is empty.

Header transition remains:

1. `X-Organization-ID` is canonical.
2. `X-Org-Id` is a temporary compatibility alias.
3. `X-Tenant-ID` is a temporary legacy alias.

All three are untrusted selectors until the authenticated organization
resolver validates organization and membership state.

### Authenticated Admin/API

Priority:

1. Organization aktif yang terikat pada session.
2. Explicit organization selector dari trusted header hanya untuk user yang sudah diautentikasi.
3. Default membership jika hanya satu organization.

Rules:

- Header canonical: `X-Organization-ID`.
- `X-Org-Id` menjadi compatibility alias sementara.
- `X-Tenant-ID` tidak menjadi canonical business header.
- Header tidak dipercaya sebelum membership/status diverifikasi.
- Session menyimpan active organization dan membership version.
- Database hanya menerima active context dari membership aktif yang dimiliki user session.
- Membership version pada session adalah snapshot; perubahan membership membuat snapshot stale dan harus ditolak resolver.
- Switch organization memperbarui session; access token lama harus diputar atau context diverifikasi server-side.
- Super admin tidak otomatis menjadi anggota semua tenant.

### Public Landing Page

Priority:

1. Verified custom domain.
2. Platform-managed subdomain.
3. Platform fallback route dengan organization/page slug.

Rules:

- Public request tidak menerima organization ID header dari browser.
- Host dinormalisasi dan dibandingkan dengan trusted domain registry.
- Forwarded host hanya dipercaya dari configured reverse proxy.
- Platform marketing host diarahkan ke platform organization.
- Disabled/suspended organization tidak melayani public page.

### Internal Service/Worker

- Event wajib membawa `organization_id`.
- Worker memuat organization context sebelum memproses event.
- Unknown/inactive organization membuat event gagal terkontrol.
- Internal header hanya diterima dari authenticated service identity/network.

## Organization Lifecycle

Statuses:

```txt
pending
active
suspended
disabled
archived
provisioning
provisioning_failed
```

Lifecycle:

1. Create organization.
2. Reserve slug dan default subdomain.
3. Create owner membership.
4. Assign plan/module entitlement.
5. Provision default settings/branding.
6. Activate.
7. Suspend/restore bila billing, abuse, atau security membutuhkan.
8. Archive dan deprovision dengan retention policy.

Status effects:

| Status | Admin API | Public Site | Background Jobs |
| --- | --- | --- | --- |
| `active` | Allowed | Allowed | Allowed |
| `pending` | Setup-only | Disabled | Provisioning only |
| `suspended` | Restricted/read-only | Configurable maintenance response | Critical jobs only |
| `disabled` | Denied except platform operator | Disabled | Denied |
| `archived` | Platform operator only | Disabled | Retention jobs only |

## Membership

Membership statuses:

```txt
invited
active
suspended
removed
```

Capabilities:

- User belongs to multiple organizations.
- One organization has one or more owners.
- Invitation and acceptance.
- Member list, role assignment, suspend, remove, and transfer ownership.
- Default organization preference.
- Last active organization per session.
- Membership version increments on role/status changes for cache invalidation.

Membership removal or suspension must invalidate active organization sessions immediately.

## Authorization

Authorization decision requires:

```txt
authenticated user
+ active membership
+ organization status
+ permission
+ permission scope
+ resource ownership when relevant
+ feature entitlement
```

Platform operator capabilities use explicit permissions:

```txt
platform.organization.read
platform.organization.manage
platform.organization.suspend
platform.organization.impersonate
platform.organization.provision
platform.domain.manage
platform.audit.read
```

Organization permissions:

```txt
organization.read
organization.update
organization.member.read
organization.member.manage
organization.role.manage
organization.domain.manage
organization.audit.read
organization.billing.read
organization.feature.read
```

Cross-tenant access must never be inferred only from `super_admin` role name.

## Operator Impersonation

Impersonation is optional but should be designed safely:

- Explicit start/stop operation.
- Require permission and reason/ticket.
- Short-lived impersonation session.
- Banner/metadata visible to admin frontend.
- Audit both operator identity and effective organization/user.
- Sensitive operations may remain prohibited during impersonation.
- No silent tenant switching.

Impersonation disimpan terpisah dari active organization operator. Record
menyimpan operator session/user, target organization/user optional, reason,
ticket reference, expiry, stop actor/reason, dan metadata. Satu operator
session hanya boleh memiliki satu impersonation yang belum ditutup.
Impersonation tidak boleh melampaui expiry session operator. Jika target user
diisi, user tersebut harus memiliki membership aktif pada target organization.

## Domain and Host Management

Organization domain types:

```txt
platform
subdomain
custom
```

Domain state:

```txt
pending
verified
active
failed
disabled
```

Requirements:

- Canonical host uniqueness.
- DNS verification challenge.
- Primary host and redirect aliases.
- SSL provisioning status.
- Domain reassignment requires ownership re-verification.
- Prevent host header injection and cache poisoning.
- Reserved platform subdomains cannot be claimed by customer organizations.

Landing Page may own page-to-domain mapping, while organization owns tenant host identity and verification policy. Boundary final must avoid duplicate domain registries.

## Feature Entitlement

Tenant module access is controlled by entitlement:

```txt
organization_id
feature_key
source
status
limits
effective_from
effective_until
```

Sources:

```txt
plan
addon
trial
platform_override
```

Semua source disimpan sebagai row terpisah agar histori plan, addon, trial, dan
override tetap dapat diaudit. Effective entitlement ditentukan oleh repository
dan service berdasarkan status, time window, source precedence, dan version;
schema tidak menghapus source yang kalah precedence.

`platform_override` wajib memiliki actor, reason, dan `effective_until`.
`limits` selalu berupa JSON object tervalidasi dan tidak menerima scalar atau
array pada root.

Examples:

```txt
landing.enabled
landing.max_pages
landing.custom_domain
landing.analytics
pos.enabled
crm.enabled
```

Permission answers "may this user do it". Entitlement answers "has this organization purchased/enabled it". Both must pass.

## Quota and Usage

- Hard limits enforced transactionally where possible.
- Soft limits return warnings before blocking.
- Usage counters can be materialized but source data remains authoritative.
- Usage counter memakai key organization, feature, metric, dan explicit period.
- Limits use organization timezone/billing period.
- Platform organization can use explicit internal plan, not hidden unlimited branches.

## Data Access Contract

Repository rules:

- Tenant-scoped repository method requires organization scope.
- No generic `FindByID(id)` for tenant data; use `FindByID(ctx, organizationID, id)`.
- Create payload receives organization from context, not request body.
- Update/delete verifies row organization in the query.
- Bulk operations reject mixed-organization IDs.
- Cross-tenant repository lives behind explicit platform admin interface.
- Raw SQL review checks every tenant-scoped query.

Service rules:

- Validate organization active and entitlement.
- Do not accept client organization as authoritative.
- Publish events with organization ID.
- Audit mutations with organization and actor.

## PostgreSQL RLS

Recommended shared-schema defense:

```sql
SET LOCAL app.organization_id = '<uuid>';
```

Policy concept:

```sql
USING (organization_id = current_setting('app.organization_id', true)::uuid)
WITH CHECK (organization_id = current_setting('app.organization_id', true)::uuid)
```

Rules:

- App runtime role must not have `BYPASSRLS`.
- Platform migration/maintenance role is separate.
- Missing organization session variable fails closed.
- Connection pool transaction always resets local state.
- RLS rollout starts on new/high-risk tables, then expands after integration tests.

## Caching and Redis

- Cache key includes `organization_id`.
- Domain cache includes canonical host and registry version.
- Permission and entitlement cache includes membership/entitlement version.
- Session active organization update invalidates relevant cache.
- Pub/sub or version check handles multi-instance invalidation.
- No shared unscoped list cache for tenant data.

## Storage Isolation

- Object key prefix includes organization ID.
- Signed upload/download validates organization ownership.
- Public asset URL maps only published assets.
- Storage metadata remains tenant-scoped.
- Dedicated bucket is optional enterprise placement.
- Moving organization storage placement requires migration state and audit.

## Events, Jobs, and Notification

Every tenant business event includes:

```txt
event_id
event_type
organization_id
actor_user_id optional
resource_type
resource_id
occurred_at
payload
```

- Idempotency key is scoped by organization.
- Job scheduler partitions/filters by organization.
- Retry and dead-letter preserve organization context.
- Notification preference/log/outbox must not mix organization IDs.

## Audit and Observability

Structured logs include:

```txt
request_id
organization_id
organization_slug
user_id
membership_id
resolution_source
impersonation_session_id
```

Audit minimum:

- Organization create/update/status.
- Member invitation/status/role.
- Active organization switch.
- Domain verification.
- Entitlement override.
- Dedicated placement change.
- Operator cross-tenant access/impersonation.

Metrics must avoid uncontrolled organization label cardinality. Per-tenant detail belongs in logs or bounded analytics.

## Security Baseline

- Never trust tenant header without authenticated membership.
- Validate UUID/slug/host canonical form.
- Do not reveal whether inaccessible organization/resource exists.
- Fail closed when context is missing.
- Prevent mass assignment of `organization_id`.
- Separate platform operator and organization member authorization.
- Rate limit switch, invitation, domain verify, and password-protected public access.
- Encrypt dedicated DB/storage/provider credential references.
- Audit data export and cross-tenant operations.
- Backup/restore must preserve tenant boundaries.

## Data Residency, Backup, and Deletion

- Organization stores region/data placement metadata.
- Shared database supports logical tenant export.
- Dedicated database supports per-tenant backup/restore.
- Archive uses retention window before destructive purge.
- Legal hold prevents purge.
- Deletion workflow covers database, storage, cache, search index, and provider data.
- Platform organization cannot be deleted through normal customer flow.

## Landing Page Integration

### Platform Marketing Landing Page

- Owned by platform organization.
- Resolved from platform primary host, for example `zyad.cloud`.
- Managed by authorized platform marketing users.
- Leads are organization-scoped to platform organization.
- Platform landing analytics, media, forms, and notification remain isolated from customer tenants.

### Customer Landing Page

- Owned by customer organization.
- Resolved from verified subdomain/custom domain or fallback route.
- Access requires `landing.*` permission and relevant entitlement.
- Suspended organization public behavior follows organization lifecycle policy.

`LAND-0003` depends on the multi-tenant context, membership, authorization, and repository guard tasks in this document.

## Existing Conflicts and Decisions

| Existing State | Decision |
| --- | --- |
| README uses `X-Tenant-ID` | Canonical authenticated selector becomes `X-Organization-ID` |
| Code uses `X-Org-Id` without validation | Keep only as temporary alias; resolver validates membership |
| JWT has no active organization | Session/context gains active organization; token strategy decided in auth integration task |
| README suggests database-per-tenant | Keep as enterprise target; shared schema is MVP |
| Organization module is placeholder | It becomes owner of organization lifecycle and membership business rules |
| Landing baseline uses `owner_type` | New design uses concrete `organization_id`, including platform organization |
| Auth docs contain planned USER-0401 | Multi-tenant tasks become canonical implementation plan; Auth/User docs link to them |

## Non-Goals for MVP

- Full cross-region active-active tenancy.
- Automatic dedicated database provisioning across cloud vendors.
- Arbitrary nested tenant hierarchy.
- Fully dynamic ABAC expression engine.
- Customer-controlled database credentials.
- Silent platform operator access.
