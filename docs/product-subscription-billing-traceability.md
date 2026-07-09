# Traceability: Product/Catalog, Subscription, Billing

> Index requirement → file kode → API → permission → status untuk 3 modul hasil refactor. Requirement asal
> ada di `docs/reference-plan-billing-subscribe.md` dan `docs/billing-plan-concept-reference.md` (tetap
> dipertahankan sebagai riwayat).

## Modul: product (`internal/modules/product/`)

| Requirement | File kode | API | Permission | DB | Status |
|---|---|---|---|---|---|
| Kelola master fitur platform | `model/feature.go`, `repository/feature_repository.go`, `service/feature_service.go` | `GET/POST/PATCH /platform/product/features*` | `platform.product.feature.read/manage` | `product_features` | Refactor |
| Kelola katalog plan | `model/plan.go`, `repository/plan_repository.go`, `service/plan_service.go` | `GET/POST/PATCH/DELETE /platform/product/plans*` | `platform.product.plan.read/manage` | `product_plans` | Refactor |
| Kelola harga plan | `repository/plan_repository.go` (price methods), `service/plan_service.go` | `GET/POST/PATCH/DELETE /platform/product/plans/:id/prices*` | `platform.product.plan_price.read/manage` | `product_plan_prices` | Refactor |
| Kelola entitlement per plan | `repository/plan_entitlement_repository.go`, `service/plan_entitlement_service.go` | `GET/PUT /platform/product/plans/:id/entitlements` | `platform.product.entitlement.read/manage` | `product_plan_entitlements` | Refactor |

## Modul: subscription (`internal/modules/subscription/`)

| Requirement | File kode | API | Permission | DB | Status |
|---|---|---|---|---|---|
| Kelola subscription organization (admin) | `model/subscription.go`, `repository/subscription_repository.go`, `service/subscription_service.go` | `GET/POST/PATCH /platform/subscriptions*`, `POST /platform/subscriptions/:id/{cancel,suspend}` | `platform.subscription.read/manage` | `customer_subscriptions` | Refactor |
| Audit trail perubahan subscription | `model/event.go`, `repository/subscription_repository.go` (CreateEvent) | — (internal, muncul lewat response subscription) | — | `subscription_events` | Refactor |
| Sinkronisasi entitlement plan → organization | `repository/entitlement_sink.go` (pindah dari billing) | — (internal, dipanggil service) | — | `organization_entitlements` (existing, tidak berubah) | Refactor |
| Guard feature/quota untuk modul lain (landing, organization) | `service/subscription_guard_service.go` (rename dari `billing_guard_service.go`) | — (dipakai sebagai dependency internal) | — | baca `customer_subscriptions` + `organization_entitlements` | Refactor |

## Modul: billing (`internal/modules/billing/`, ramping)

| Requirement | File kode | API | Permission | DB | Status |
|---|---|---|---|---|---|
| Kelola invoice (admin) | `model/invoice.go`, `repository/invoice_repository.go`, `service/invoice_service.go` | `GET/POST /platform/billing/invoices`, `POST /platform/billing/invoices/:id/mark-paid` | `platform.billing.invoice.read/manage`, `platform.billing.payment.manage` | `billing_invoices`, `billing_invoice_items` | Tetap |
| Kelola payment & webhook event | `model/payment.go`, `repository/payment_repository.go`, `service/payment_service.go` | — (webhook belum diwire ke route, lihat catatan di bawah) | — | `billing_payments`, `billing_payment_events` | Tetap, WIP (webhook belum lengkap) |
| Self-service billing tenant (composition: plan+subscription+invoice) | `service/tenant_billing_service.go`, `handler/tenant_billing_handler.go` | `GET /app/billing/current-plan`, `GET /app/billing/usage`, `GET /app/billing/invoices`, `POST /app/billing/upgrade`, `POST /app/billing/cancel` | `organization.billing.read/manage` | baca lintas modul (product+subscription+billing) | Tetap, route TIDAK berubah |

## Catatan Status

- **Refactor**: struktur/nama berubah (tabel, package, permission, route) tapi logika bisnis tidak berubah.
- **Tetap**: tidak tersentuh refactor ini sama sekali.
- **WIP**: sudah diketahui belum lengkap sebelum refactor ini (webhook payment provider — lihat
  `docs/next-development-tasks.md` P1-3), status ini tidak berubah oleh refactor domain split.

## Migration Index

| Migration | Isi | Menggantikan |
|---|---|---|
| `000058_create_product_catalog` | `product_features`, `product_plans`, `product_plan_prices`, `product_plan_entitlements` | `000050_create_billing_plan_catalog` |
| `000059_create_customer_subscriptions` | `customer_subscriptions`, `subscription_events` | `000051_create_billing_subscriptions` + bagian subscription dari `000053_create_billing_events` |
| `000060_drop_old_billing_plan_subscription_tables` | Drop 6 tabel lama | — |
| `000061_seed_product_catalog` | Seed features/plans/prices/entitlements ke tabel baru | `000055_seed_billing_features_plans`, `000056_seed_billing_plan_entitlements` |
| `000062_rename_billing_permissions` | Update 8 permission slug | `000054_seed_billing_permissions` (parsial), `000057_seed_billing_plan_price_permissions` |

Migration 000050–000057 dan 000054/000057 **tidak dihapus** — tetap ada sebagai riwayat migration,
hasil akhirnya dikoreksi oleh migration baru di atas (pattern: correct via new migration, jangan edit
migration lama, sesuai `AGENTS.md`).
