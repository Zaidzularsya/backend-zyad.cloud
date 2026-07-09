# Rencana Refactor: Product/Catalog, Subscription, Billing

> Checklist eksekusi teknis untuk memisahkan `internal/modules/billing/` menjadi 3 modul. Lihat
> `docs/product-subscription-billing-concept.md` untuk alasan/konsep, dan
> `docs/product-subscription-billing-traceability.md` untuk pelacakan requirement→file→API→permission.

## 1. Migration Baru (`migrations/`)

Tidak mengedit migration 000050–000057 yang sudah ada — semua koreksi lewat migration baru, mulai
000058 (nomor pasti dikonfirmasi ulang saat eksekusi via `ls migrations | sort | tail -1`).

| # | File | Isi |
|---|---|---|
| 1 | `000058_create_product_catalog.up/down.sql` | Create `product_features`, `product_plans`, `product_plan_prices`, `product_plan_entitlements` — schema identik migration 000050, nama tabel/constraint disesuaikan |
| 2 | `000059_create_customer_subscriptions.up/down.sql` | Create `customer_subscriptions`, `subscription_events` — schema identik migration 000051 & bagian subscription di 000053 |
| 3 | `000060_drop_old_billing_plan_subscription_tables.up/down.sql` | `DROP TABLE` 6 tabel lama (urutan dependency: entitlements→features/prices/plans, events→subscriptions). `down.sql` me-recreate persis seperti 000050/000051 untuk rollback |
| 4 | `000061_seed_product_catalog.up/down.sql` | Pindahkan seed dari 000055 (27 features, 5 plans, 8 prices) & 000056 (~124 entitlements) ke tabel baru, pola `ON CONFLICT` sama |
| 5 | `000062_rename_billing_permissions.up/down.sql` | `UPDATE permissions SET permission_name=..., slug=..., module=...` untuk 8 slug yang pindah domain (lihat tabel di concept doc) — **bukan** delete+insert, agar `role_permissions` tidak putus |

## 2. Struktur Modul Go Baru

```
internal/modules/product/
├── model/          # Feature, Plan, PlanPrice, PlanEntitlement (pindah dari billing/model/{plan,feature}.go)
├── repository/      # PlanRepository, FeatureRepository, PlanEntitlementRepository (update nama tabel SQL)
├── service/         # PlanService, FeatureService, PlanEntitlementService
├── handler/          # PlatformProductHandler (pecahan dari platform_billing_handler.go, route /platform/product/*)
├── dto/              # request/response DTO untuk plan/feature/price/entitlement
├── errors.go         # kode error khusus product (PLAN_NOT_FOUND, FEATURE_NOT_FOUND, dst)
└── routes.go

internal/modules/subscription/
├── model/          # Subscription, SubscriptionEvent (pindah dari billing/model/{subscription,event}.go)
├── repository/      # SubscriptionRepository, entitlement_sink.go (sync ke organization_entitlements)
├── service/         # SubscriptionService, SubscriptionGuardService (rename dari BillingGuardService)
├── handler/          # PlatformSubscriptionHandler (pecahan platform_billing_handler.go, route /platform/subscriptions*)
├── dto/
├── errors.go         # SUBSCRIPTION_NOT_FOUND, SUBSCRIPTION_INACTIVE, SUBSCRIPTION_SUSPENDED, QUOTA_EXCEEDED, dst
└── routes.go

internal/modules/billing/        (ramping)
├── model/          # Invoice, InvoiceItem, Payment, PaymentEvent (TETAP)
├── repository/      # InvoiceRepository, PaymentRepository (TETAP)
├── service/         # InvoiceService, PaymentService, TenantBillingService (composition layer)
├── handler/          # PlatformBillingHandler (ramping: invoice+mark-paid saja), TenantBillingHandler (TETAP)
├── dto/
├── errors.go         # INVOICE_NOT_FOUND, PAYMENT_ALREADY_PROCESSED, dst (TETAP)
└── routes.go
```

`TenantBillingService`/`TenantBillingHandler` (route `/api/v1/app/billing/*`, TIDAK berubah) menjadi
composition layer: memanggil interface publik dari `product` (info plan) dan `subscription` (status
langganan, upgrade/cancel) selain invoice miliknya sendiri.

## 3. Checklist Wiring (`internal/app/`)

- [ ] `app.go`: ganti konstruktor `billingrepo.NewPlanRepository` → `productrepo.NewPlanRepository`,
      `billingrepo.NewFeatureRepository` → `productrepo.NewFeatureRepository`,
      `billingrepo.NewPlanEntitlementRepository` → `productrepo.NewPlanEntitlementRepository`,
      `billingrepo.NewSubscriptionRepository` → `subscriptionrepo.NewSubscriptionRepository`,
      `billingservice.NewPlanService/NewFeatureService/NewPlanEntitlementService` → `productservice.*`,
      `billingservice.NewSubscriptionService` → `subscriptionservice.*`,
      `billingservice.NewBillingGuardService` → `subscriptionservice.NewSubscriptionGuardService`,
      `billinghandler.NewPlatformBillingHandler` dipecah jadi `producthandler.NewPlatformProductHandler` +
      `subscriptionhandler.NewPlatformSubscriptionHandler` + `billinghandler.NewPlatformBillingHandler`
      (ramping, hanya invoice).
- [ ] `dependency.go`: field `PlatformBillingHandler` dipecah jadi `PlatformProductHandler`,
      `PlatformSubscriptionHandler`, `PlatformBillingHandler` (ramping).
- [ ] `router.go`: register 3 handler baru dengan prefix masing-masing
      (`/api/v1/platform/product`, `/api/v1/platform/subscriptions`, `/api/v1/platform/billing`).
- [ ] `module_routes.go`: pastikan `product.RegisterRoutes`/`subscription.RegisterRoutes` (jika ada
      public route) ikut terdaftar seperti pola modul lain.
- [ ] Cek referensi cross-module: `organizationservice.WithMembershipBillingGuard(billingGuardService)` dan
      `WithDomainBillingGuard(billingGuardService)` di `app.go` — parameter berganti ke
      `subscriptionGuardService` (tipe berubah, method interface sama, jadi cukup ganti variabel).

## 4. Checklist Test

- [ ] Pindahkan `plan_service_test.go`, `plan_entitlement_service_test.go`, bagian feature dari
      `billing_repository_integration_test.go` → `internal/modules/product/`.
- [ ] Pindahkan `subscription_service_test.go`, bagian subscription dari
      `billing_repository_integration_test.go` dan `tenant_billing_service_test.go` → `internal/modules/subscription/`
      (bagian invoice/payment dari `tenant_billing_service_test.go` tetap di `billing`).
- [ ] Sisakan test invoice/payment (`invoice_service_test.go`, `payment_service_test.go`,
      `tenant_billing_handler_test.go`) di `internal/modules/billing/`.
- [ ] Update semua stub/mock interface di test yang mengacu nama lama (`billing.PlanNotFoundError()` dst)
      ke package baru.

## 5. Checklist OpenAPI (`api/openapi.yaml`)

- [ ] Rename 13 path `/api/v1/platform/billing/{plans,features,subscriptions}*` sesuai mapping domain baru.
- [ ] **Tidak mengubah** 5 path `/api/v1/app/billing/*`.
- [ ] Rename schema component `Billing{Plan,PlanPrice,Feature,PlanEntitlement,Subscription}*` →
      `Product{Plan,PlanPrice,Feature,PlanEntitlement}*` / `CustomerSubscription*` (daftar lengkap di
      concept doc bagian "Rename OpenAPI Schema Component").
- [ ] **Tidak mengubah** schema `BillingInvoice*`, `BillingPayment*`.
- [ ] Update parameter `BillingPlanID`/`BillingFeatureID`/`BillingSubscriptionID` → `ProductPlanID`/
      `ProductFeatureID`/`SubscriptionID` (tetap `BillingInvoiceID` untuk invoice).

## 6. Checklist Dokumentasi

- [ ] Tambah catatan riwayat di bagian atas `docs/billing-plan-concept-reference.md`,
      `docs/billing-plan-development-tasks.md`, `docs/billing-plan-development-traceability.md`,
      `docs/reference-plan-billing-subscribe.md` menunjuk ke 3 dokumen baru sebagai struktur final.
- [ ] Update `docs/module-map.md`, `docs/database-context.md`, `docs/permission-context.md`,
      `docs/api-contract-review.md`, `docs/development-traceability.md`, `docs/next-development-tasks.md`
      agar mencerminkan modul `product`/`subscription`/`billing` (bukan `billing` tunggal).

## 7. Follow-up Frontend (referensi saja — TIDAK dieksekusi dalam paket kerja ini)

Repo terpisah `frontend.zyad.cloud` sudah punya integrasi nyata ke route yang berubah. Task ini
**sengaja tidak dikerjakan sekarang** (keputusan eksplisit pemilik repo) — dicatat di sini agar tidak
hilang konteksnya untuk pekerjaan berikutnya:

| File | Perubahan yang dibutuhkan |
|---|---|
| `src/features/billing/api/platform-billing.api.ts` | Ganti path `/platform/billing/plans*`, `/platform/billing/features*` → `/platform/product/*`; `/platform/billing/subscriptions*` → `/platform/subscriptions*` |
| `src/features/billing/api/billing.api.ts` | Tidak berubah (`/app/billing/*` tetap) |
| `src/features/billing/pages/PlatformBillingPlansPage.vue` | Mengikuti perubahan `platform-billing.api.ts` |
| `src/features/billing/pages/PlanUpgradePage.vue` | Tidak berubah |
| Query hooks `platform-billing.queries.ts` | Mengikuti perubahan endpoint |

## 8. Verifikasi Akhir

```bash
# Setelah migration baru
go run ./cmd/migrate -direction up -dir migrations -steps 0

# Setelah refactor kode
go build ./...
go vet ./...
go test ./...
go test -tags=integration ./internal/modules/product/... ./internal/modules/subscription/... ./internal/modules/billing/...

# Sanity check tidak ada referensi tabel lama tersisa di kode/migration baru
grep -rn "billing_plans\b\|billing_features\b\|billing_plan_prices\b\|billing_plan_entitlements\b\|billing_subscriptions\b\|billing_subscription_events\b" internal/ migrations/000058*.sql migrations/000059*.sql migrations/000061*.sql migrations/000062*.sql
# harus kosong (referensi lama hanya boleh ada di migration 000050-000057 dan down.sql milik 000060)
```
