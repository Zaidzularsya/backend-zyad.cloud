# Development Traceability — Index Gabungan

> Dokumen ini adalah **index lintas-modul**: requirement/fitur → kode → API → database → permission → test.
> Untuk detail traceability mendalam per fitur, dokumen khusus sudah ada dan lebih lengkap — dokumen ini
> hanya merangkum dan menaut ke sana agar bisa dibaca sebagai satu gambaran utuh.

## Cara Pakai

Baca baris sesuai modul yang sedang dikerjakan, lalu buka dokumen traceability detail (`*-traceability-index.md`)
untuk requirement dan status task granular.

## Modul: user (Auth & User Management)

| Aspek | Detail |
|---|---|
| Requirement | `docs/reference-auth-user.md`, `docs/google-auth-development-tasks.md` |
| File kode utama | `internal/modules/user/{handler,service,repository,model,dto,seeder}` |
| API | `/api/v1/auth/*`, `/api/v1/users/me*`, `/api/v1/admin/users/*` — kontrak publik: `docs/auth-user-public-api-contract.md` |
| DB | `users`, `user_profiles`, `auth_identities`, `sessions`, `refresh_tokens`, `login_histories`, `audit_logs`, `roles`, `permissions`, `role_permissions`, `user_roles`, `user_permissions` |
| Permission | `user.*`, `role.*`, `permission.*`, `audit.read` — detail: [permission-context.md](permission-context.md) |
| Status | **Stabil** |
| Traceability detail | `docs/auth-user-traceability-index.md`, `docs/auth-user-migration-seed-plan.md`, `docs/google-auth-traceability-index.md` |
| Catatan | Seeder `super_admin.go` adalah source of truth permission dasar |

## Modul: organization (Multi-Tenant Core)

| Aspek | Detail |
|---|---|
| Requirement | `docs/reference-multi-tenant.md` |
| File kode utama | `internal/modules/organization/{handler,service,repository,model,dto,seeder}` |
| API | `/api/v1/organization*`, `/api/v1/platform/organizations*`, `/api/v1/users/me/organizations`, `/api/v1/users/me/switch-organization` |
| DB | `organizations`, `organization_domains`, `organization_memberships`, `organization_entitlements`, `organization_usage_counters`, `organization_impersonation_sessions`, `reserved_subdomains` |
| Permission | `organization.user.*`, `organization.billing.*`, `organization.domain.manage` |
| Status | **Stabil** |
| Traceability detail | `docs/multi-tenant-traceability-index.md`, `docs/multi-tenant-schema-query-audit.md`, `docs/multi-tenant-rls.md`, `docs/multi-tenant-development-tasks.md` |
| Catatan | RLS **tidak** diterapkan di tabel modul ini — lihat [database-context.md](database-context.md) bagian 3 |

## Modul: landing (Landing Page Builder)

| Aspek | Detail |
|---|---|
| Requirement | `docs/reference-landing-page.md` |
| File kode utama | `internal/modules/landing/{domain,service,repository,handler,dto,routes}` |
| API | `/api/v1/admin/landing-pages*`, `/api/v1/admin/landing/*`, `/api/v1/admin/landing-submissions*`, `/api/v1/public/landing/*` — kontrak publik: `docs/landing-page-public-api-contract.md` |
| DB | 20 tabel `landing_*` — lihat [database-context.md](database-context.md) bagian 1, audit skema: `docs/landing-page-schema-audit.md` |
| Permission | `landing.page.*`, `landing.section.manage`, `landing.form.manage`, `landing.media.manage`, `landing.submission.*` |
| Status | **Stabil, aktif dikembangkan** (banyak unit test baru 2026-06-30) |
| Traceability detail | `docs/landing-page-traceability-index.md`, `docs/landing-page-development-tasks.md` |
| Catatan | Satu-satunya modul dengan RLS penuh; `internal/modules/landingpage/` (folder kosong) bukan modul ini |

## Modul: product, subscription, billing (hasil refactor domain-split, 2026-07-01)

> Modul `billing` tunggal (dibuat 2026-06-29/30) di-refactor 2026-07-01 menjadi 3 domain. Detail lengkap:
> [product-subscription-billing-traceability.md](product-subscription-billing-traceability.md) (traceability
> khusus 3 modul ini) dan [product-subscription-billing-concept.md](product-subscription-billing-concept.md)
> (konsep & alasan pemisahan). Ringkasan:

| Aspek | product | subscription | billing (ramping) |
|---|---|---|---|
| Requirement | `docs/reference-plan-billing-subscribe.md` (riwayat) | `docs/reference-plan-billing-subscribe.md` (riwayat) | `docs/reference-plan-billing-subscribe.md` (riwayat) |
| File kode utama | `internal/modules/product/{model,service,repository,handler,dto}` | `internal/modules/subscription/{model,service,repository,handler,dto}` | `internal/modules/billing/{model,service,repository,handler,dto}` |
| API | `/api/v1/platform/product/*` | `/api/v1/platform/subscriptions*` | `/api/v1/platform/billing/invoices*`, `/api/v1/app/billing/*` (TIDAK berubah) |
| DB | `product_features`, `product_plans`, `product_plan_prices`, `product_plan_entitlements` | `customer_subscriptions`, `subscription_events` | `billing_invoices`, `billing_invoice_items`, `billing_payments`, `billing_payment_events` |
| Permission | `platform.product.*` | `platform.subscription.*` | `platform.billing.invoice.*`, `platform.billing.payment.manage`, `organization.billing.*` (semua TIDAK berubah) |
| Status | **Refactor selesai** (2026-07-01) | **Refactor selesai** (2026-07-01) | **Refactor selesai** (ramping) — payment gateway (Xendit) masih belum lengkap terintegrasi (status pra-refactor, tidak berubah) |
| Catatan | Katalog global, tidak organization-scoped | Sinkron entitlement wajib ke `organization_entitlements`/`organization_usage_counters` — jangan buat source of truth kedua (lihat `AGENTS.md`) | `TenantBillingService` jadi composition layer memanggil `product`+`subscription` untuk route `/app/billing/*` |

## Modul: notification (Core, Lintas Modul)

| Aspek | Detail |
|---|---|
| Requirement | `docs/reference-notification.md` |
| File kode utama | `internal/core/notification/{handler,service,repository,template,dispatcher,publisher,consumer,domain}` |
| API | Endpoint manajemen template/preference — lihat handler di `internal/core/notification/handler/` (belum di-cross-check detail ke OpenAPI, lihat [api-contract-review.md](api-contract-review.md)) |
| DB | `notification_templates`, `notification_logs`, `notification_preferences`, `notification_outbox_events` |
| Status | Terpakai lintas modul (dipicu oleh event dari user/organization/billing) |
| Traceability detail | `docs/notification-traceability-index.md`, `docs/notification-development-tasks.md` |
| Catatan | Worker terpisah (`cmd/worker`) memproses outbox — email selalu aktif, WhatsApp/Discord tergantung provider terkonfigurasi |

## Modul Stub (Belum Ada Traceability)

13 modul (`account`, `asset`, `contract`, `dashboard`, `landingpage`, `mikrotik`, `newsaggregator`, `order`,
`payment`, `product`, `provisioning`, `radius`, `resource`) belum punya requirement/API/DB/test — lihat
[module-map.md](module-map.md) untuk checklist sebelum mulai menggarap salah satunya, dan buat traceability
doc baru mengikuti pola tabel di atas begitu modul mulai diimplementasikan.

## Traceability Lintas-Modul yang Perlu Diperhatikan

- **landing → organization**: cek entitlement (via subscription) dan domain binding
  (`landing_domain_bindings` ↔ `organization_domains`) sebelum publish page.
- **subscription → organization**: `SubscriptionService` sinkron ke `organization_entitlements` via
  interface `subscriptionEntitlementSink` (`internal/modules/subscription/repository/entitlement_sink.go`)
  — perubahan skema product/subscription yang mempengaruhi entitlement harus disertai pengecekan dampak ke
  `organization` module.
- **subscription → product**: `SubscriptionService` membaca `product_plan_entitlements` (via
  `PlanEntitlementService`) sebelum sinkron ke `organization_entitlements` — perubahan skema entitlement
  plan harus disertai pengecekan dampak ke `subscription`.
- **user → semua modul**: `AuthenticatedUser` (roles/permissions) dari JWT dipakai oleh semua permission
  middleware — perubahan struktur claims token berdampak ke semua modul yang protected.
