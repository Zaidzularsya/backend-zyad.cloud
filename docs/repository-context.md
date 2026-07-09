# Repository Context — zyad.cloud Backend

> Dokumen ini adalah titik masuk pertama untuk memahami repository ini. Ditulis berdasarkan pembacaan kode
> aktual (bukan asumsi dari dokumentasi lama), per 2026-07-01. Untuk detail per topik, dokumen ini merujuk
> ke dokumen lain di `docs/` alih-alih mengulang isinya.

## 1. Ringkasan Repository

`zyad.cloud` adalah backend Go untuk platform SaaS multi-tenant. Modul yang **benar-benar berjalan** saat
ini: autentikasi & manajemen user (`user`), manajemen organisasi/tenant (`organization`), landing page
builder (`landing`), dan billing/subscription (`billing`, masih WIP). Ada 13 modul lain di
`internal/modules/` yang baru berupa scaffold kosong (lihat [module-map.md](module-map.md)) — modul-modul
ini **bukan bug yang belum selesai**, melainkan slot domain yang memang belum digarap.

`README.md` versi lama menyebut modul CRM, POS, Membership, dan penggunaan Casbin/Goth/Redis-session yang
**tidak ada di kode**. README.md sudah dikoreksi sebagai bagian dari paket dokumentasi ini (lihat riwayat
git `README.md`) — visi bisnis lama dipindah ke section roadmap, bagian fitur/arsitektur/auth diganti agar
sesuai kode aktual.

## 2. Stack Teknologi

| Komponen | Pilihan | Sumber |
|---|---|---|
| Bahasa | Go 1.26 | `go.mod` |
| Web framework | Gin v1.10.0 (`github.com/gin-gonic/gin`) | `go.mod` |
| Database driver | pgx/v5 (`github.com/jackc/pgx/v5`), **tanpa ORM** — SQL langsung | `go.mod`, `internal/platform/database` |
| Cache/session | go-redis v9 (`github.com/redis/go-redis/v9`) | `go.mod` |
| Password hashing | `golang.org/x/crypto/bcrypt` | `internal/core/auth` |
| Validasi request | go-playground/validator v10 | `go.mod`, `internal/core/validation` |
| Testing | testify v1.11 (`github.com/stretchr/testify`) | `go.mod` |
| Logging | `log/slog` (standar Go, dibungkus `internal/platform/logger`) | kode |
| Konfigurasi | custom loader (`os.Getenv` + parsing `.env`), **bukan** viper/envconfig | `internal/config` |
| Migration | custom migration runner, **bukan** golang-migrate/Flyway | `internal/platform/database/migration`, `cmd/migrate` |
| Payment gateway | Xendit (`internal/platform/xendit`) | kode, WIP di modul billing |
| Notifikasi | Email (SMTP/noop), WhatsApp (provider terkonfigurasi/noop), Discord | `internal/platform/mail`, `internal/platform/whatsapp`, `internal/core/notification` |

Tidak ditemukan Dockerfile, docker-compose, Makefile, atau CI workflow (`.github/workflows`) di root
repository — lihat [development-guide.md](development-guide.md) untuk cara setup lokal tanpa alat-alat ini.

## 3. Struktur Folder

```
api/                    openapi.yaml (spec utama, ~6968 baris) + openapi-landing.yaml
cmd/
├── api/main.go         entry point HTTP server
├── migrate/main.go     runner migration (flag: -direction up|down, -dir, -steps)
├── seed/main.go        runner seed (flag: -name super-admin | platform-organization)
└── worker/main.go      notification worker (flag: -once untuk single batch)

internal/
├── app/                 bootstrap aplikasi, dependency injection manual
│   ├── app.go            App struct + New()/Run() — wiring ~50+ dependency (repository/service/handler)
│   ├── dependency.go      struct Dependencies yang dioper ke router
│   ├── router.go          registrasi route + middleware global (Gin)
│   ├── server.go          lifecycle HTTP server (graceful shutdown)
│   └── module_routes.go   registrasi RegisterRoutes() semua modul di internal/modules
│
├── config/               config loader custom (env var + .env), lihat struct Config di config.go
│
├── core/                 fondasi lintas modul (BUKAN "platform infrastruktur eksternal")
│   ├── auth/              JWT (HMAC-SHA256), Google OAuth verify, password hashing
│   ├── cache/             abstraksi cache
│   ├── crypto/            token/random string generator
│   ├── errors/             tipe error standar + mapping ke HTTP response
│   ├── event/              publish/subscribe event internal
│   ├── http/               helper response (OK/Error envelope)
│   ├── idempotency/        pencegah duplicate request (order/payment)
│   ├── middleware/         Authenticate, ResolveAuthenticatedOrganization, ResolvePublicOrganization,
│   │                       RequireActiveTenant, CORS, RequestID, Recovery, rate limit
│   ├── notification/       sistem notifikasi (handler/service/repository/dispatcher/publisher/consumer)
│   ├── permission/         RBAC: handler, service, repository, middleware (Require/RequireOrganization)
│   ├── tenant/             tipe Context multi-tenant (OrganizationID, ResolutionSource, dll)
│   └── validation/          custom Gin validator
│
├── platform/             adapter infrastruktur eksternal
│   ├── database/           pool pgx, migration runner, tenant_transaction.go, RLS integration test
│   ├── logger/, mail/, metrics/, mikrotik/, redis/, storage/, whatsapp/, xendit/
│
├── modules/              modul domain bisnis — lihat detail lengkap di module-map.md
│   ├── landing/           STABIL — landing page builder (canonical, bukan landingpage)
│   ├── landingpage/       folder KOSONG (0 file) — sisa scaffold lama, jangan dipakai
│   ├── organization/      STABIL — multi-tenant core
│   ├── billing/           WIP — plan/subscription/invoice/payment/entitlement
│   ├── user/              STABIL — auth + user management
│   └── account, asset, contract, dashboard, mikrotik, newsaggregator, order, payment,
│       product, provisioning, radius, resource — STUB (scaffold kosong)
│
└── shared/               pagination, response envelope, utils (string/time/slug/phone)

migrations/              57 pasang file .up/.down.sql, penomoran 000001–000057 (lihat database-context.md)
docs/                    dokumentasi per-fitur existing (25 file) + 8 dokumen konteks baru (paket ini)
scripts/, tests/         script pendukung & test tambahan
```

**Catatan struktur tidak konsisten yang ditemukan:**
- `internal/modules/landingpage/` adalah folder kosong, sedangkan modul aktif bernama `internal/modules/landing/`.
  Berpotensi membingungkan developer baru. Rekomendasi ada di [next-development-tasks.md](next-development-tasks.md).
- Beberapa modul stub (`payment` di `internal/modules/` vs fungsi payment gateway yang justru diimplementasikan
  di dalam `internal/modules/billing/` + `internal/platform/xendit/`) — ada potensi tumpang tindih tanggung
  jawab yang perlu diklarifikasi saat modul `payment` mulai digarap.

## 4. Arsitektur

- **Modular monolith**, satu binary (`cmd/api`), bukan microservices.
- **Layering per modul**: `handler` (HTTP binding, Gin) → `service` (business rule, transaction boundary)
  → `repository` (akses data, SQL langsung via pgx) → PostgreSQL. Aturan ini didokumentasikan di `AGENTS.md`
  dan konsisten diterapkan di modul `landing`, `organization`, `billing`, `user`.
- **Dependency Injection manual** — tidak memakai wire/fx. Semua service/repository/handler dikonstruksi
  eksplisit di `internal/app/app.go` dan dioper via struct `Dependencies` (`internal/app/dependency.go`) ke
  `newRouter()`.
- **Multi-tenant**: shared database + Row-Level Security Postgres, tapi **RLS baru diterapkan ke tabel
  `landing_*`** (lihat [database-context.md](database-context.md) untuk detail dan risiko). Tabel
  `organization_*`, `billing_*`, dan tabel user/auth **tidak** memakai RLS — isolasi tenant di tabel-tabel
  itu bergantung pada filter `organization_id` di level repository/query, bukan enforcement database.
  Resolusi tenant context ada 3 jalur (session, public host/subdomain, worker/internal) — detail di
  [permission-context.md](permission-context.md).
- **RBAC custom** (bukan Casbin) — 3 layer: `roles ↔ role_permissions ↔ permissions`, `user_roles`
  (global atau per-organization), plus override langsung `user_permissions` (allow/deny). Detail di
  [permission-context.md](permission-context.md).
- **Event/notifikasi**: pola outbox (`notification_outbox_events`) + worker terpisah (`cmd/worker`) yang
  polling dan dispatch ke channel (email/WhatsApp/Discord/in-app).

## 5. Module Map (ringkas)

Lihat [module-map.md](module-map.md) untuk tabel lengkap 18 modul. Ringkasan status:

| Status | Modul |
|---|---|
| Stabil | `user`, `organization` |
| Stabil, aktif dikembangkan | `landing` |
| WIP (sangat baru) | `billing` |
| Stub/scaffold kosong | `account`, `asset`, `contract`, `dashboard`, `landingpage`, `mikrotik`, `newsaggregator`, `order`, `payment`, `product`, `provisioning`, `radius`, `resource` |

## 6. Flow Utama Aplikasi

**Request masuk (authenticated):**
```
HTTP request → middleware.Authenticate() [validasi JWT access token dari header Authorization]
  → middleware.ResolveAuthenticatedOrganization() [tentukan organization_id dari session/selector]
  → permission middleware (Require/RequireOrganization) [cek permission slug]
  → handler [bind request, panggil service]
  → service [business rule, transaction, panggil repository]
  → repository [SQL ke Postgres, discope by organization_id / RLS untuk tabel landing_*]
  → response envelope (internal/shared/response)
```

**Request publik (landing page delivery):**
```
HTTP request (Host header) → middleware.ResolvePublicOrganization() [resolve org dari subdomain/custom domain]
  → middleware.RequireActiveTenant()
  → public landing handler → service → repository (RLS-scoped)
```

**Login:**
```
POST /auth/login → AuthService.Login() → verifikasi bcrypt → buat session (tabel sessions) →
  generate access token (JWT HS256, TTL configurable) + refresh token (disimpan hash-nya di sessions/
  refresh_tokens) → catat login_histories
```

## 7. Dependency Eksternal Penting

- **PostgreSQL** — database utama, wajib.
- **Redis** — **Confirmed: currently used for session storage only.** Pemakaian sebagai cache umum, queue,
  atau rate limiting adalah *future improvement* yang belum diimplementasikan — lihat
  [development-guide.md](development-guide.md) dan tabel Ringkasan Verifikasi Manual di bagian 10.
- **Xendit** — payment gateway untuk billing (`XENDIT_*` env), integrasi belum lengkap (lihat
  [next-development-tasks.md](next-development-tasks.md)).
- **SMTP** (opsional, fallback ke `noop` jika `MAIL_HOST` kosong) — pengiriman email notifikasi.
- **WhatsApp/Discord provider** (opsional, default `noop`) — channel notifikasi tambahan.
- **Google OAuth** (opsional, `AUTH_GOOGLE_ENABLED`) — login via Google.

## 8. Indeks Dokumentasi

**Dokumen konteks baru (paket ini):**
- [development-guide.md](development-guide.md) — setup lokal, env var, command development
- [module-map.md](module-map.md) — mapping lengkap 18 modul
- [api-contract-review.md](api-contract-review.md) — kesesuaian OpenAPI vs kode
- [database-context.md](database-context.md) — schema, migration, RLS, seed
- [permission-context.md](permission-context.md) — auth, RBAC, middleware
- [development-traceability.md](development-traceability.md) — index traceability lintas modul
- [next-development-tasks.md](next-development-tasks.md) — task lanjutan terprioritas
- [product-subscription-billing-concept.md](product-subscription-billing-concept.md) — konsep domain
  Product/Subscription/Billing (struktur final setelah refactor 2026-07-01)
- [product-subscription-billing-refactor-plan.md](product-subscription-billing-refactor-plan.md) —
  checklist teknis refactor
- [product-subscription-billing-traceability.md](product-subscription-billing-traceability.md) —
  traceability modul product/subscription/billing

**Dokumen existing per-fitur (jangan diduplikasi, rujuk saja):**
- Auth/User: `reference-auth-user.md`, `auth-user-development-tasks.md`, `auth-user-traceability-index.md`,
  `auth-user-public-api-contract.md`, `auth-user-migration-seed-plan.md`, `google-auth-*.md`
- Landing Page: `reference-landing-page.md`, `landing-page-development-tasks.md`,
  `landing-page-traceability-index.md`, `landing-page-public-api-contract.md`, `landing-page-schema-audit.md`
- Multi-tenant: `reference-multi-tenant.md`, `multi-tenant-development-tasks.md`,
  `multi-tenant-traceability-index.md`, `multi-tenant-schema-query-audit.md`, `multi-tenant-rls.md`
- Billing (riwayat sebelum refactor domain-split): `reference-plan-billing-subscribe.md`,
  `billing-plan-concept-reference.md`, `billing-plan-development-tasks.md`,
  `billing-plan-development-traceability.md`
- Notification: `reference-notification.md`, `notification-development-tasks.md`,
  `notification-traceability-index.md`
- Umum: `migration-guide.md`

`AGENTS.md` mendefinisikan urutan baca ("workflow") per fitur yang mengarah ke dokumen-dokumen di atas —
gunakan itu saat mengerjakan fitur spesifik. Dokumen di bagian "Dokumen konteks baru" di atas untuk
gambaran lintas-modul.

## 9. Catatan Penting untuk Claude/Developer Berikutnya

1. **Jangan asumsikan modul stub sebagai bug.** 11 dari 18 modul di `internal/modules/` memang belum
   digarap (plus `landingpage` yang legacy/kosong) — cek [module-map.md](module-map.md) sebelum mulai kerja
   di modul manapun.
2. **`internal/modules/landingpage/` adalah folder mati** — modul kanonik untuk landing page adalah
   `internal/modules/landing/`.
3. **Modul `billing` sudah di-refactor (2026-07-01) menjadi 3 modul**: `product` (katalog: plan/feature/
   harga/entitlement), `subscription` (subscription aktif + sinkron entitlement), `billing` (ramping:
   invoice/payment saja). Tabel `billing_plans`/`billing_features`/`billing_plan_prices`/
   `billing_plan_entitlements`/`billing_subscriptions`/`billing_subscription_events` **sudah tidak ada**,
   digantikan `product_*`/`customer_subscriptions`/`subscription_events`. Lihat
   [product-subscription-billing-concept.md](product-subscription-billing-concept.md).
4. **RLS hanya melindungi tabel `landing_*`.** Tabel `organization_*`, `product_*`, `customer_subscriptions`,
   `billing_*`, dan tabel user/auth mengandalkan filter `organization_id` manual di level repository —
   kalau menulis query baru di modul selain landing, pastikan filter tenant tidak lupa ditambahkan manual.
5. **Seed harus berurutan**: `super-admin` dulu, baru `platform-organization` (yang kedua mencari user
   super-admin yang sudah ada via `findPlatformOwner`). Lihat [database-context.md](database-context.md).
6. **README.md lama (sebelum koreksi) menyebut fitur yang tidak ada** (CRM, POS, Membership, Casbin,
   Goth, SSO). Jangan jadikan acuan requirement tanpa cross-check ke kode.
7. Tidak ada Docker/Makefile/CI di repo ini — semua command dijalankan langsung via `go run`/`go build`.
   Lihat [development-guide.md](development-guide.md).

## 10. Ringkasan Verifikasi Manual (dikonfirmasi 2026-07-01)

Beberapa poin yang sebelumnya ditandai "Perlu verifikasi" di paket dokumentasi ini sudah dikonfirmasi
lewat keputusan manual dari pemilik repo. Tabel ini adalah rangkuman lintas-dokumen — detail teknis tetap
ada di masing-masing dokumen terkait (link di kolom terakhir).

| Topic | Previous Status | Manual Confirmation | Updated Documentation Status | Required Follow-up |
|---|---|---|---|---|
| Redis usage scope | Perlu verifikasi — cakupan pemakaian Redis (cache/session/queue) belum jelas | Redis currently used for session storage only | **Confirmed** — cache/queue/rate-limit adalah future improvement, bukan implementasi saat ini | Tidak ada aksi kode segera; evaluasi ulang saat kebutuhan cache muncul |
| golangci-lint config | Perlu verifikasi — tidak ditemukan file config saat riset awal | golangci-lint akan dipakai sebagai quality gate; belum dikonfirmasi ada config in-repo | **Confirmed (absence verified 2026-07-01)** — dicek `.golangci.{yml,yaml,toml,json}` di root: tidak ada satupun. golangci-lint is intended to be used, but in-repo configuration needs verification or creation | Buat config minimal `.golangci.yml` (lihat [next-development-tasks.md](next-development-tasks.md)) |
| RBAC granular scope (`own`/`team`/`branch`/`department`) | Perlu verifikasi — apakah scope selain organization/all dicek di runtime | Scope sudah ada di skema database (termasuk `organization` dan `all`); enforcement runtime belum dikonfirmasi | **Needs code verification** — RBAC scope exists in database schema, including organization and all, but runtime enforcement needs code verification | Audit `internal/core/permission/policy/checker.go` (lihat [permission-context.md](permission-context.md)) |
| `data_placement=dedicated` (database-per-tenant) | Perlu verifikasi — status implementasi mekanisme dedicated belum dikonfirmasi | Fase sekarang fokus shared database; dedicated database-per-tenant ditunda | **Deferred (Future capability)** — current target implementation is shared database; dedicated database-per-tenant is deferred as future capability | Tidak ada aksi kode segera; fokus audit isolasi `organization_id`/`tenant_id` di shared DB (lihat [database-context.md](database-context.md)) |
| `api/openapi-landing.yaml` vs `api/openapi.yaml` | Perlu verifikasi — relasi kedua file belum jelas | `openapi-landing.yaml` harus digabung ke `openapi.yaml`; `openapi.yaml` jadi source of truth tunggal | **Confirmed decision, merge pending** — api/openapi.yaml is the canonical OpenAPI source; landing OpenAPI definitions should be merged into api/openapi.yaml | Lakukan merge fisik + hapus/arsipkan stub file (lihat [api-contract-review.md](api-contract-review.md), [next-development-tasks.md](next-development-tasks.md)) |

Poin "Perlu verifikasi" lain yang **belum** ada keputusan manual (mis. lokasi pasti handler endpoint tertentu,
status binary `api_bin`/folder `bin/`, apakah migration runner membungkus transaksi otomatis) tetap berstatus
**Needs code verification** — lihat dokumen masing-masing untuk detail.
