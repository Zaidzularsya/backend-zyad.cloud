# Billing Plan Development Tasks

> **Catatan riwayat (refactor domain-split)**: Breakdown di dokumen ini sudah selesai dieksekusi
> dan modulnya kemudian di-refactor menjadi 3 domain (Product/Catalog, Subscription, Billing) — lihat
> [product-subscription-billing-refactor-plan.md](product-subscription-billing-refactor-plan.md) untuk
> checklist refactor tersebut. Dokumen ini dipertahankan sebagai riwayat breakdown kerja awal.

Dokumen ini adalah breakdown development untuk:

```txt
internal/modules/billing
```

Sumber:

- `README.md`
- `AGENTS.md`
- `docs/reference-plan-billing-subscribe.md`
- `docs/billing-plan-concept-reference.md`
- `docs/reference-multi-tenant.md`
- `docs/multi-tenant-development-tasks.md`
- `docs/multi-tenant-traceability-index.md`
- `docs/landing-page-development-tasks.md`
- `docs/migration-guide.md`

## Tujuan

Membangun module billing yang mengelola plan catalog, feature catalog,
subscription, invoice, payment record, dan sinkronisasi entitlement untuk
organization. Module ini harus mengikuti arsitektur existing: handler hanya
HTTP binding, service memegang business rule/transaction/idempotency,
repository hanya database access, dan semua perubahan database melalui
migration.

## Development Rules

- Jangan implementasi billing logic berdasarkan nama plan.
- Runtime feature access harus memakai entitlement dan quota.
- Jangan membuat module `subscription` terpisah untuk MVP; subscription berada
  di `internal/modules/billing`.
- Jangan membuat `tenant_id` column bisnis baru; gunakan `organization_id`.
- Jangan membuat runtime entitlement table kedua pada MVP. Gunakan
  `organization_entitlements` existing.
- Jangan membuat runtime usage counter table kedua pada MVP. Gunakan
  `organization_usage_counters` existing.
- Jangan menambah dependency production tanpa konfirmasi.
- Jangan mengubah arsitektur utama route/dependency tanpa konfirmasi.
- Endpoint final wajib disinkronkan ke `api/openapi.yaml`.
- Database memakai schema `public` dan migration biasa.
- Tenant-owned billing table harus dipertimbangkan untuk RLS dengan helper
  existing.

## Existing State

| Area | Status |
| --- | --- |
| `internal/modules/billing` | Core module, handler, service, repository, DTO, dan model sudah tersedia. |
| Plan catalog | Sudah ada dengan plan price, feature, dan plan entitlement management. |
| Subscription table | Sudah ada dan dipakai untuk current plan, suspend/cancel, dan upgrade activation. |
| Invoice/payment table | Sudah ada untuk invoice item, payment record, dan mark-paid flow. |
| Runtime entitlement | Sudah ada di `organization_entitlements`. |
| Runtime usage counter | Sudah ada di `organization_usage_counters`. |
| Platform permission | Sudah disediakan oleh `000054_seed_billing_permissions`. |
| Tenant billing permission | `organization.billing.read` dan `organization.billing.manage` sudah disediakan oleh seed billing permission. |
| Landing feature gate | `AccessPolicy` ada dan billing MVP mempertahankan prefix feature key existing `landing.*`. |
| Migration terakhir | Billing saat ini memakai `000050` sampai `000057`. |

## Recommended Folder Structure

Ikuti instruksi `AGENTS.md` dan pola module existing:

```txt
internal/modules/billing/
  handler/
  service/
  repository/
  dto/
  model/
  routes.go
  errors.go
  doc.go
```

Jika ingin memakai folder `domain/` seperti module Landing, pastikan keputusan
itu konsisten dan tidak memecah model persistence secara membingungkan. Untuk
MVP billing, `model/` lebih selaras dengan `AGENTS.md` dan module organization.

## Phase 0 - Review Existing Architecture

### BILL-0001: Review Multi-Tenant and Billing Boundary

Status: `done`

Description:

- Review `README.md`, `AGENTS.md`, dan dokumen billing ini.
- Review `docs/reference-multi-tenant.md`.
- Review `docs/multi-tenant-traceability-index.md`.
- Pastikan organization adalah tenant canonical dan billing account.

Files impacted:

- None.

Implementation notes:

- Catat bahwa `organization_entitlements` dan `organization_usage_counters`
  sudah ada.
- Jangan membuat desain yang menabrak source of truth existing.

Acceptance criteria:

- Developer dapat menjelaskan boundary billing vs organization.
- Tidak ada tabel/runtime guard duplikat.

Dependencies:

- None.

Test scenario:

- Documentation review checklist selesai.

### BILL-0002: Review Route, Permission, and Handler Pattern

Status: `done`

Description:

- Review `internal/app/router.go`.
- Review handler organization dan landing.
- Review permission middleware.
- Tentukan route billing protected di bawah `/api/v1`.

Files impacted:

- None pada review.

Implementation notes:

- Billing protected handler sebaiknya di-wire di `internal/app`, seperti
  organization/landing.
- `billing.RegisterRoutes` saat ini dipanggil pada public module group dan
  masih no-op. Jangan menaruh billing tenant/admin route sensitif di public
  module group.

Acceptance criteria:

- Route plan jelas: platform admin, tenant dashboard, dan optional internal.
- Permission list awal disepakati.

Dependencies:

- `BILL-0001`.

Test scenario:

- Review route existing dan catat path final.

### BILL-0003: Review Migration and Seed Convention

Status: `done`

Description:

- Review `docs/migration-guide.md`.
- Review migration existing `000013` sampai `000049`.
- Review seed SQL permission dan template existing.
- Review `cmd/seed/main.go` untuk seed command yang berbasis Go.

Files impacted:

- None pada review.

Implementation notes:

- Migration terakhir saat dokumen dibuat: `000049`.
- Gunakan migration SQL untuk catalog/permission seed.
- Go seed command hanya jika seed butuh config/runtime seperti platform
  organization.

Acceptance criteria:

- Nomor migration berikutnya tidak bentrok.
- Down migration punya urutan FK benar.

Dependencies:

- None.

Test scenario:

- `rg --files migrations | sort` menunjukkan urutan final.

## Phase 1 - Database Migration

### BILL-DB-001: Billing Plan Catalog Tables

Status: `done`

Description:

- Buat migration plan catalog:
  - `billing_plans`
  - `billing_plan_prices`
  - `billing_features`
  - `billing_plan_entitlements`

Files impacted:

- `migrations/000050_create_billing_plan_catalog.up.sql`
- `migrations/000050_create_billing_plan_catalog.down.sql`

Implementation notes:

- Semua ID UUID.
- `billing_plans` dan `billing_plan_prices` soft delete dengan `deleted_at`.
- `billing_features` tidak perlu soft delete di MVP; gunakan `is_active`.
- Gunakan `varchar + check constraint`.
- Gunakan `numeric(14,2)` untuk amount.
- Gunakan `jsonb` untuk metadata/limits.
- Unique active plan code memakai partial unique index.

Acceptance criteria:

- Up/down migration tersedia.
- Unique dan check constraint jelas.
- Down drop table berurutan: plan entitlements, features, plan prices, plans.

Dependencies:

- `BILL-0003`.

Test scenario:

- `go run ./cmd/migrate -direction up -steps 1`
- `go run ./cmd/migrate -direction down -steps 1`

### BILL-DB-002: Billing Subscription Table

Status: `done`

Description:

- Buat `billing_subscriptions`.

Files impacted:

- `migrations/000051_create_billing_subscriptions.up.sql`
- `migrations/000051_create_billing_subscriptions.down.sql`

Implementation notes:

- FK ke `organizations(id)` dan `billing_plans(id)`.
- Status: `trialing`, `active`, `past_due`, `grace_period`, `suspended`,
  `canceled`, `expired`.
- Billing interval: `monthly`, `yearly`, `custom`.
- Partial unique satu usable subscription per organization.
- Tenant-owned table: apply RLS jika sudah mengikuti convention table lain.

Acceptance criteria:

- Satu organization tidak dapat punya dua subscription usable.
- Query by organization/status terindeks.

Dependencies:

- `BILL-DB-001`.
- `migrations/000013_create_organizations`.

Test scenario:

- Insert duplicate active subscription gagal.
- Insert active + canceled subscription untuk organization sama berhasil.

### BILL-DB-003: Billing Invoice and Payment Tables

Status: `done`

Description:

- Buat:
  - `billing_invoices`
  - `billing_invoice_items`
  - `billing_payments`

Files impacted:

- `migrations/000052_create_billing_invoices_payments.up.sql`
- `migrations/000052_create_billing_invoices_payments.down.sql`

Implementation notes:

- Invoice dan payment punya `organization_id`.
- Invoice item cascade delete saat invoice rollback/draft dihapus.
- Payment provider reference unique jika tersedia.
- Jangan percaya amount dari client; service recalculates invoice total.

Acceptance criteria:

- Invoice number unique.
- Payment provider reference idempotent.
- Amount memakai numeric, bukan float.

Dependencies:

- `BILL-DB-002`.

Test scenario:

- Duplicate invoice number gagal.
- Duplicate provider reference gagal.
- Tenant invoice query memakai organization filter.

### BILL-DB-004: Billing Event Tables

Status: `done`

Description:

- Buat:
  - `billing_subscription_events`
  - `billing_payment_events`

Files impacted:

- `migrations/000053_create_billing_events.up.sql`
- `migrations/000053_create_billing_events.down.sql`

Implementation notes:

- Subscription event mencatat old/new status, actor, dan metadata.
- Payment event menyimpan provider payload dan provider event ID.
- Provider event ID unique per provider jika tidak null.

Acceptance criteria:

- Status transition bisa diaudit.
- Payment webhook duplicate dapat dideteksi.

Dependencies:

- `BILL-DB-002`.
- `BILL-DB-003`.

Test scenario:

- Duplicate `(provider, provider_event_id)` gagal.
- Event list by invoice/subscription terindeks.

### BILL-DB-005: RLS Review for Billing Tables

Status: `done`

Description:

- Tentukan table mana yang platform-global dan tenant-owned.
- Apply RLS untuk tenant-owned table jika sesuai helper existing.

Files impacted:

- Migration billing yang relevan.
- Optional integration test RLS.

Implementation notes:

- Platform-global: plan catalog dan feature catalog.
- Tenant-owned: subscriptions, invoices, payments, subscription events,
  payment events.
- Plan entitlements adalah catalog/control-plane dan tidak tenant-owned.

Acceptance criteria:

- Runtime tenant tidak dapat membaca invoice organization lain.
- Platform admin path tetap explicit dan audited.

Dependencies:

- `migrations/000019_add_rls_foundation`.

Test scenario:

- Integration test runtime role tanpa context gagal membaca tenant-owned table.

## Phase 2 - Seed Data

### BILL-SEED-001: Seed Billing Permissions

Status: `done`

Description:

- Seed platform billing permission dan tenant billing permission.

Files impacted:

- `migrations/000054_seed_billing_permissions.up.sql`
- `migrations/000054_seed_billing_permissions.down.sql`

Implementation notes:

Platform permissions:

- `platform.billing.plan.read`
- `platform.billing.plan.manage`
- `platform.billing.feature.read`
- `platform.billing.feature.manage`
- `platform.billing.entitlement.read`
- `platform.billing.entitlement.manage`
- `platform.billing.subscription.read`
- `platform.billing.subscription.manage`
- `platform.billing.invoice.read`
- `platform.billing.invoice.manage`
- `platform.billing.payment.manage`

Tenant permissions:

- `organization.billing.read`
- `organization.billing.manage`

Acceptance criteria:

- Seed idempotent.
- `super_admin` mendapat permission platform.
- Organization owner/admin mendapat tenant billing permission sesuai role
  convention existing.

Dependencies:

- Permission tables existing.

Test scenario:

- Seed dijalankan dua kali tanpa duplicate.

### BILL-SEED-002: Seed Features, Plans, and Prices

Status: `done`

Description:

- Seed default billing features, plans, dan plan prices.

Files impacted:

- `migrations/000055_seed_billing_features_plans.up.sql`
- `migrations/000055_seed_billing_features_plans.down.sql`

Implementation notes:

Default plans:

- `free`
- `starter`
- `growth`
- `business`
- `enterprise`

Default feature catalog mengikuti `docs/billing-plan-concept-reference.md`.

Prices MVP:

- Free monthly IDR 0.
- Starter monthly/yearly placeholder.
- Growth monthly/yearly placeholder.
- Business monthly/yearly placeholder.
- Enterprise custom amount 0/inactive or custom interval.

Acceptance criteria:

- Seed idempotent.
- Feature key stable lowercase.
- Seed tidak menghapus custom plan.

Dependencies:

- `BILL-DB-001`.

Test scenario:

- Run up seed twice, count tetap stabil.

### BILL-SEED-003: Seed Plan Entitlements

Status: `done`

Description:

- Seed entitlement default untuk Free, Starter, Growth, Business, Enterprise.

Files impacted:

- `migrations/000056_seed_billing_plan_entitlements.up.sql`
- `migrations/000056_seed_billing_plan_entitlements.down.sql`

Implementation notes:

Free sample:

- `users.max_users = 1`
- `landing.enabled = true`
- `landing.max_pages = 1`
- `landing.custom_domain = false`
- `crm.enabled = false`
- `media.max_storage_mb = 100`

Starter sample:

- `users.max_users = 3`
- `landing.enabled = true`
- `landing.max_pages = 3`
- `landing.custom_domain = false`
- `crm.enabled = true`
- `crm.max_contacts = 500`
- `media.max_storage_mb = 1000`

Growth sample:

- `users.max_users = 10`
- `landing.max_pages = 10`
- `landing.custom_domain = true`
- `domain.max_custom_domains = 1`
- `crm.max_contacts = 5000`
- `media.max_storage_mb = 5000`
- `whatsapp.enabled = true`
- `whatsapp.max_messages_per_month = 1000`

Business sample:

- `users.max_users = 25`
- `landing.max_pages = 50`
- `domain.max_custom_domains = 5`
- `crm.max_contacts = 25000`
- `pos.enabled = true`
- `membership.enabled = true`
- `automation.enabled = true`

Enterprise:

- Customizable. Seed can set high/null limit, but runtime custom tenant should
  use platform override or custom contract.

Acceptance criteria:

- Seed idempotent.
- Down migration only removes known seeded rows, not custom user rows.
- Enterprise custom override tidak tertimpa.

Dependencies:

- `BILL-SEED-002`.

Test scenario:

- Re-run seed and verify no duplicate entitlement rows.

## Phase 3 - Domain, DTO, and Errors

### BILL-BE-001: Billing Models

Status: `done`

Description:

- Buat model untuk plan, plan price, feature, plan entitlement, subscription,
  invoice, invoice item, payment, subscription event, payment event.

Files impacted:

- `internal/modules/billing/model/*.go`

Implementation notes:

- Model tidak bergantung pada Gin atau pgx.
- Status dan type constants punya `IsValid()`.
- Amount tetap decimal-friendly; hindari float di domain public.

Acceptance criteria:

- Model selaras migration.
- Unit test enum validation tersedia.

Dependencies:

- `BILL-DB-001..004`.

Test scenario:

- `go test ./internal/modules/billing/model`

### BILL-BE-002: Billing DTO

Status: `done`

Description:

- Buat request/response DTO untuk platform admin dan tenant dashboard.

Files impacted:

- `internal/modules/billing/dto/request.go`
- `internal/modules/billing/dto/response.go`

Implementation notes:

- Request dan response dipisah.
- Internal raw provider payload tidak diekspos ke tenant.
- Pagination mengikuti pola existing.

Acceptance criteria:

- DTO punya binding/validation tag yang sesuai.
- Tenant response tidak membocorkan invoice organization lain.

Dependencies:

- `BILL-BE-001`.

Test scenario:

- DTO mapping unit test untuk response critical.

### BILL-BE-003: Billing Error Constants

Status: `done`

Description:

- Definisikan error billing standar.

Files impacted:

- `internal/modules/billing/errors.go`

Implementation notes:

Error codes minimal:

- `SUBSCRIPTION_NOT_FOUND`
- `SUBSCRIPTION_INACTIVE`
- `SUBSCRIPTION_SUSPENDED`
- `FEATURE_NOT_ENABLED`
- `QUOTA_EXCEEDED`
- `PLAN_NOT_FOUND`
- `FEATURE_NOT_FOUND`
- `INVOICE_NOT_FOUND`
- `PAYMENT_ALREADY_PROCESSED`
- `INVALID_BILLING_STATUS`

Acceptance criteria:

- Error memakai `internal/core/errors`.
- Error code stabil untuk frontend.

Dependencies:

- None.

Test scenario:

- Unit test mapping HTTP status untuk error utama.

## Phase 4 - Repository

### BILL-REPO-001: Plan Repository

Status: `done`

Description:

- Implement CRUD/list plan dan price.

Files impacted:

- `internal/modules/billing/repository/plan_repository.go`

Implementation notes:

- Filter `deleted_at is null`.
- Query memakai context.
- Repository tidak menghitung business rule upgrade.

Acceptance criteria:

- List pagination tersedia.
- Soft delete tidak menghapus historical subscription.

Dependencies:

- `BILL-BE-001`.

Test scenario:

- Unit/integration repository create/list/update/soft delete.

### BILL-REPO-002: Feature and Plan Entitlement Repository

Status: `done`

Description:

- Implement feature catalog dan plan entitlement repository.

Files impacted:

- `internal/modules/billing/repository/feature_repository.go`
- `internal/modules/billing/repository/plan_entitlement_repository.go`

Implementation notes:

- Upsert plan entitlements atomik.
- Validate feature exists.
- Tidak menyentuh organization entitlement runtime.

Acceptance criteria:

- Satu plan-feature hanya satu row.
- Bulk replace entitlement aman dalam transaction service.

Dependencies:

- `BILL-BE-001`.

Test scenario:

- Bulk replace tidak menyisakan duplicate.

### BILL-REPO-003: Subscription Repository

Status: `done`

Description:

- Implement repository subscription lifecycle.

Files impacted:

- `internal/modules/billing/repository/subscription_repository.go`

Implementation notes:

- Query tenant-scoped by `organization_id`.
- `FindUsableByOrganization` memakai status usable.
- Row lock untuk status transition.

Acceptance criteria:

- Active subscription lookup deterministik.
- Concurrent status transition aman.

Dependencies:

- `BILL-BE-001`.

Test scenario:

- Duplicate active subscription ditolak oleh DB.

### BILL-REPO-004: Invoice and Payment Repository

Status: `done`

Description:

- Implement invoice, invoice item, payment, dan payment event repository.

Files impacted:

- `internal/modules/billing/repository/invoice_repository.go`
- `internal/modules/billing/repository/payment_repository.go`

Implementation notes:

- Invoice/payment query tenant-scoped.
- Payment event insert memakai unique provider event ID.
- Mark paid harus row-lock invoice.

Acceptance criteria:

- Tenant tidak bisa mengambil invoice organization lain.
- Provider event duplicate mapped ke idempotent result.

Dependencies:

- `BILL-BE-001`.

Test scenario:

- Mark paid dua kali tidak menggandakan payment/event.

### BILL-REPO-005: Entitlement Sink Adapter

Status: `done`

Description:

- Buat adapter repository/service kecil untuk menulis plan entitlement ke
  `organization_entitlements`.

Files impacted:

- `internal/modules/billing/repository/entitlement_sink.go`
- atau interface terhadap `internal/modules/organization/repository`

Implementation notes:

- Reuse `organizationrepo.EntitlementRepository` jika memungkinkan.
- Jangan duplikasi query besar tanpa alasan.
- Source harus `plan`.
- Source reference harus subscription ID.

Acceptance criteria:

- Sync entitlement memakai table existing.
- Non-plan sources tidak tertimpa.

Dependencies:

- `migrations/000016_create_organization_entitlements`.

Test scenario:

- Sync plan menghasilkan row `organization_entitlements` dengan source `plan`.
- Expire plan entitlement menutup entitlement aktif untuk subscription terkait.
- Sync plan tetap memakai precedence evaluation existing milik organization entitlement.

## Phase 5 - Service

### BILL-SVC-001: Plan and Feature Services

Status: `done`

Description:

- Implement PlanService, FeatureService, dan PlanEntitlementService.

Files impacted:

- `internal/modules/billing/service/plan_service.go`
- `internal/modules/billing/service/feature_service.go`
- `internal/modules/billing/service/plan_entitlement_service.go`

Implementation notes:

- Service validate status/value type.
- Bulk entitlement replacement memakai transaction existing di repository.

Acceptance criteria:

- CRUD plan/feature berjalan.
- Plan entitlement value sesuai feature type.

Dependencies:

- `BILL-REPO-001..002`.

Test scenario:

- Invalid value type ditolak.
- Matching feature value type diteruskan ke repository.

### BILL-SVC-002: Subscription Service

Status: `done`

Description:

- Implement create, activate, suspend, cancel, upgrade, downgrade schedule.

Files impacted:

- `internal/modules/billing/service/subscription_service.go`

Implementation notes:

- Service mengorkestrasi create/update, event write, dan entitlement sync.
- Atomic transaction lintas repository perlu repository transaction adapter lanjutan.
- Status transition validate old/new status.
- Tulis `billing_subscription_events`.
- Panggil entitlement sync saat activation/plan change.

Acceptance criteria:

- Subscription activation membuat entitlement snapshot.
- Invalid status transition ditolak.
- Semua transition tercatat.

Dependencies:

- `BILL-REPO-003`.
- `BILL-REPO-005`.

Test scenario:

- Create subscription active -> entitlement plan tersedia di evaluation existing.
- Invalid status transition ditolak sebelum update repository.

### BILL-SVC-003: Invoice Service

Status: `done`

Description:

- Implement invoice create/list/detail dan total calculation.

Files impacted:

- `internal/modules/billing/service/invoice_service.go`

Implementation notes:

- Jangan percaya total dari client.
- Invoice number generated deterministic enough and unique.
- Invoice item total dihitung service.

Acceptance criteria:

- Invoice total benar.
- Tenant invoice list scoped.

Dependencies:

- `BILL-REPO-004`.

Test scenario:

- Create invoice with items calculates subtotal/tax/discount/total.
- Negative total ditolak.

### BILL-SVC-004: Payment Service

Status: `done`

Description:

- Implement manual mark-paid dan record payment event.

Files impacted:

- `internal/modules/billing/service/payment_service.go`

Implementation notes:

- Idempotent mark-paid.
- If invoice already paid, return current state.
- Payment event created once.
- If paid invoice metadata indicates `upgrade_request`, activate target plan and
  resync entitlement snapshot.
- Provider webhook foundation can reuse payment-event idempotency path via
  `(provider, provider_event_id)` before invoice/payment mutation logic is
  attached.
- Later gateway webhook can reuse idempotency path.

Acceptance criteria:

- Mark-paid tidak menggandakan payment.
- Invoice paid_at dan payment paid_at konsisten.

Dependencies:

- `BILL-SVC-003`.

Test scenario:

- Call mark-paid twice and verify one payment/effective event.
- Invoice already paid returns existing paid payment without creating duplicate payment/event.
- Upgrade invoice paid updates subscription plan and runtime entitlement.

### BILL-SVC-005: Billing Guard Service

Status: `done`

Description:

- Implement runtime guard untuk subscription, feature, dan quota.

Files impacted:

- `internal/modules/billing/service/billing_guard_service.go`

Implementation notes:

- Reuse organization entitlement service untuk feature evaluation.
- Snapshot quota count bisa via small counter interfaces ke module owner.
- Periodic quota bisa via `organization_usage_counters`.
- Fail closed jika dependency guard belum tersedia.

Acceptance criteria:

- Suspended subscription blocked.
- Disabled feature blocked.
- Quota exceeded returns stable error.

Dependencies:

- `BILL-SVC-002`.
- Organization entitlement service existing.

Test scenario:

- Suspended subscription blocks landing page create.
- Disabled feature returns stable billing error.
- Quota exceeded returns stable billing error.

## Phase 6 - HTTP Handler and API

### BILL-API-001: Platform Plan and Feature API

Status: `done`

Description:

- Implement platform admin endpoints for plans, prices, features, and plan
  entitlements.

Files impacted:

- `internal/modules/billing/handler/platform_plan_handler.go`
- `internal/modules/billing/handler/platform_feature_handler.go`
- `internal/modules/billing/handler/platform_entitlement_handler.go`
- `internal/app/dependency.go`
- `internal/app/app.go`
- `internal/app/router.go`

Implementation notes:

- Route under `/api/v1/platform/billing`.
- Require platform tenant and platform billing permission.
- Use response helpers existing.

Acceptance criteria:

- Non-platform tenant cannot manage plan catalog.
- Response shape consistent.

Dependencies:

- `BILL-SVC-001`.

Test scenario:

- Handler test unauthorized, forbidden, success.

### BILL-API-002: Platform Subscription, Invoice, Payment API

Status: `done`

Description:

- Implement platform endpoints for subscription, invoice, and mark-paid.

Files impacted:

- `internal/modules/billing/handler/platform_subscription_handler.go`
- `internal/modules/billing/handler/platform_invoice_handler.go`
- `internal/modules/billing/handler/platform_payment_handler.go`
- `internal/app/*`

Implementation notes:

- Platform admin can query across organizations.
- All cross-tenant reads must require platform permission.
- Manual mark-paid must capture actor user ID.

Acceptance criteria:

- Platform admin can create subscription for organization.
- Mark-paid idempotent.

Dependencies:

- `BILL-SVC-002..004`.

Test scenario:

- Platform handler creates subscription and invoice.

### BILL-API-003: Tenant Billing Dashboard API

Status: `done`

Description:

- Implement tenant endpoints:
  - current plan
  - usage
  - invoices
  - upgrade request
  - cancel request with period-end scheduling

Files impacted:

- `internal/modules/billing/handler/tenant_billing_handler.go`
- `internal/app/*`

Implementation notes:

- Route under `/api/v1/app/billing`.
- Must require active customer tenant.
- Read requires `organization.billing.read`.
- Upgrade/cancel requires `organization.billing.manage`.

Acceptance criteria:

- Tenant only sees own billing data.
- Suspended tenant can still access billing screen.
- Cancel request keeps current plan active and only marks `cancel_at_period_end`.
- Current plan payload can include aggregate usage summary for usage-counter based features.

Dependencies:

- `BILL-SVC-002..005`.

Test scenario:

- Tenant A cannot read Tenant B invoice.
- Suspended tenant still receives current/latest billing subscription context.
- Upgrade request creates an invoice for the selected target plan.

## Phase 7 - Guard Integration

### BILL-GUARD-001: Landing Page Create Guard

Status: `done`

Description:

- Integrate subscription/feature/quota check before creating landing page.

Files impacted:

- `internal/modules/landing/service/page_service.go`
- `internal/modules/landing/service/access_policy.go`
- Related tests.

Implementation notes:

- Keep Landing business logic in Landing service.
- Billing guard should be interface-injected.
- Pertahankan feature key existing `landing.enabled` dan gunakan prefix
  `landing.*` untuk quota/fitur Landing baru.

Acceptance criteria:

- Create landing page fails if subscription suspended.
- Create landing page fails if `landing.enabled` false.
- Create landing page fails if `landing.max_pages` exceeded.

Dependencies:

- `BILL-SVC-005`.

Test scenario:

- Landing page create quota exceeded returns `QUOTA_EXCEEDED`.
- Landing page duplicate is blocked when copied sections exceed `landing.max_sections_per_page`.

### BILL-GUARD-002: Custom Domain Guard

Status: `done`

Description:

- Integrate domain entitlement check.

Files impacted:

- `internal/modules/organization/service/domain_service.go`
- `internal/modules/landing/service/domain_service.go`
- Related tests.

Implementation notes:

- Organization domain create checks `domain.enabled` and
  `domain.max_custom_domains`.
- Landing binding checks `landing.custom_domain`.

Acceptance criteria:

- Free/Starter cannot bind custom domain if entitlement false.
- Growth can bind within limit.

Dependencies:

- `BILL-SVC-005`.

Test scenario:

- Custom domain creation blocked after quota full.

### BILL-GUARD-003: User Invitation Guard

Status: `done`

Description:

- Integrate user/member limit.

Files impacted:

- `internal/modules/organization/service/membership_service.go`
- or user invitation service once finalized.

Implementation notes:

- Check `users.invite_user`.
- Check `users.max_users`.
- Keep membership rule in organization module.

Acceptance criteria:

- Invite blocked when active member count reaches limit.

Dependencies:

- `BILL-SVC-005`.

Test scenario:

- Invite user at max limit returns `QUOTA_EXCEEDED`.

## Phase 8 - OpenAPI Documentation

### BILL-DOC-001: OpenAPI Billing Contract

Status: `done`

Description:

- Update OpenAPI with billing schemas and endpoints.

Files impacted:

- `api/openapi.yaml`

Implementation notes:

- Add platform billing endpoints.
- Add tenant billing endpoints.
- Add error response examples.
- Keep existing OpenAPI valid.

Acceptance criteria:

- OpenAPI validates.
- Request/response examples are present.

Dependencies:

- `BILL-API-001..003`.

Test scenario:

- Run OpenAPI validation command if available in repo/tooling.

## Phase 9 - Testing

### BILL-TEST-001: Unit Tests

Status: `done`

Description:

- Add unit tests for services and guards.

Files impacted:

- `internal/modules/billing/service/*_test.go`
- `internal/modules/billing/model/*_test.go`

Implementation notes:

- Focus on status transition, value type validation, invoice total,
  idempotency, and guard errors.

Acceptance criteria:

- Critical business rules covered.

Dependencies:

- Service implementation.

Test scenario:

- `go test ./internal/modules/billing/...`

### BILL-TEST-002: Repository Integration Tests

Status: `done`

Description:

- Add opt-in integration tests for billing repositories.

Files impacted:

- `internal/modules/billing/repository/*_integration_test.go`

Implementation notes:

- Follow existing integration test convention with `-tags=integration`.
- Use test database only.
- Verify migration up/down and RLS where applicable.

Acceptance criteria:

- Seed idempotency verified.
- Tenant isolation verified.

Dependencies:

- Migration implementation.

Test scenario:

- `go test -tags=integration ./internal/modules/billing/repository`

### BILL-TEST-003: Cross-Module Guard Tests

Status: `done`

Description:

- Test billing guard integration with Landing/custom domain/user invitation.

Files impacted:

- Landing service tests.
- Organization service tests.
- Billing guard tests.

Implementation notes:

- Use interface fakes for guard where possible.
- Add integration test only for critical DB-backed flow.

Acceptance criteria:

- Suspended subscription blocks paid feature.
- Quota exceeded blocks create.
- RBAC and entitlement remain separate.

Dependencies:

- `BILL-GUARD-001..003`.

Test scenario:

- `go test ./internal/modules/landing/service ./internal/modules/organization/service ./internal/modules/billing/service`

## Phase 10 - Documentation Finalization

### BILL-DOC-002: Sync Documentation and Traceability

Status: `done`

Description:

- Update billing docs, README, migration guide, and traceability after
  implementation.

Files impacted:

- `docs/billing-plan-concept-reference.md`
- `docs/billing-plan-development-tasks.md`
- `docs/billing-plan-development-traceability.md`
- `README.md`
- `docs/migration-guide.md`

Implementation notes:

- Move task statuses from `planned` to `done` only after verification.
- Record implementation history.
- Keep migration numbers accurate.

Acceptance criteria:

- Docs match code and migrations.
- No stale planned item marked done prematurely.

Dependencies:

- All implementation phases.

Test scenario:

- Documentation review against code paths and migration list.

## Safe Development Order

1. Finish Phase 0 review.
2. Create migrations `000050` to `000053`.
3. Add seed permissions and default catalog `000054` to `000056`.
4. Build model/DTO/errors.
5. Build repositories with integration tests.
6. Build services and unit tests.
7. Add platform admin API.
8. Add tenant dashboard API.
9. Add billing guard service.
10. Integrate guard into Landing/custom domain/user invitation.
11. Update OpenAPI.
12. Finalize traceability and README/migration-guide status.

## Risks and Mitigation

| Risk | Mitigation |
| --- | --- |
| Duplicate entitlement source of truth | Use existing `organization_entitlements` as runtime sink. |
| Feature key drift antar dokumen dan code Landing | Pertahankan prefix existing `landing.*` pada billing seed dan guard. |
| Tenant can read other tenant invoice | Tenant-scoped repository methods, RLS, and handler context guard. |
| Payment mark-paid duplicate | Row lock invoice and unique payment/event constraints. |
| Plan changes break existing tenants | Sync snapshot entitlement per subscription, do not runtime-join plan only. |
| Overengineering billing gateway early | MVP manual payment first; provider adapter later. |
| Public route accidental exposure | Wire billing handlers in protected router, keep placeholder no-op public route. |
