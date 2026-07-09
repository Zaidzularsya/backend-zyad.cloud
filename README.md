# Multi-Tenant SaaS Platform (Golang-Based)

Platform multi-tenant berbasis **Golang** untuk landing page builder, manajemen organisasi, dan billing
berlangganan. Dokumen ini mendeskripsikan **kondisi implementasi aktual** (diverifikasi terhadap kode per
2026-07-01) — untuk gambaran arsitektur lebih dalam, baca [docs/repository-context.md](docs/repository-context.md).

---

## 🚀 Modul yang Sudah Berjalan

### 1. Multi-Tenancy (`internal/modules/organization`)
- Isolasi data lewat kolom `organization_id` di setiap tabel domain bisnis, ditambah **Row-Level Security
  PostgreSQL** khusus untuk tabel `landing_*` (tabel `organization_*`/`billing_*` mengandalkan filter
  manual di repository — lihat [docs/database-context.md](docs/database-context.md)).
- **Tenant Resolution** lewat 3 jalur: session user login, **subdomain**/**custom domain** (public host),
  dan worker/internal context. Diimplementasikan di
  `internal/modules/organization/service/{authenticated_resolver,public_host_resolver}.go` dan
  `internal/core/middleware/tenant.go`.
- Kolom `data_placement` (`shared`/`dedicated`) di tabel `organizations` menyiapkan skema untuk rencana
  database-per-tenant di masa depan. **Keputusan saat ini (Confirmed, 2026-07-01): platform memakai shared
  database untuk semua tenant.** Mode `dedicated` adalah *future capability* yang **belum** diimplementasikan
  di level koneksi database — fokus implementasi sekarang adalah isolasi lewat `organization_id`/`tenant_id`
  plus query filtering dan authorization guard yang aman. Detail: [docs/database-context.md](docs/database-context.md).

### 2. Autentikasi (`internal/modules/user`, `internal/core/auth`)
- Login **username/password** (bcrypt) dan **Google OAuth** (opsional, `AUTH_GOOGLE_ENABLED`).
- **JWT** (access + refresh token, HMAC-SHA256) — refresh token disimpan dalam bentuk hash di
  **PostgreSQL** (tabel `sessions`/`refresh_tokens`), bukan Redis.
- **Redis saat ini hanya dipakai untuk session storage** (Confirmed). Pemakaian Redis sebagai cache umum,
  queue, atau rate limiting adalah *future improvement*, belum diimplementasikan.
- Detail lengkap di [docs/permission-context.md](docs/permission-context.md).

### 3. RBAC Custom (`internal/core/permission`)
- 3 layer: `roles ↔ role_permissions ↔ permissions`, `user_roles` (global atau per-organization), dan
  override langsung `user_permissions` (allow/deny).
- Naming convention permission: `module.resource.action` (mis. `landing.page.create`,
  `organization.billing.manage`).
- Implementasi custom Go — **bukan** Casbin/go-casbin.
- Detail lengkap di [docs/permission-context.md](docs/permission-context.md).

---

## 📦 Produk yang Sudah Diimplementasikan

1. **Landing Page Builder** (`internal/modules/landing`) — modul paling matang dan aktif dikembangkan.
   Page, section, template, form/lead capture, branding, domain binding, publish/schedule, revision,
   analytics dasar. Lihat [docs/reference-landing-page.md](docs/reference-landing-page.md).
2. **Billing & Subscription** (`internal/modules/billing`) — plan catalog, subscription, invoice, payment,
   feature entitlement. **Status: WIP**, integrasi payment gateway (Xendit) belum lengkap. Lihat
   [docs/billing-plan-concept-reference.md](docs/billing-plan-concept-reference.md).
3. **Organization Management** — onboarding organisasi, membership, domain custom + verifikasi, entitlement.

13 modul lain di `internal/modules/` (`account`, `asset`, `contract`, `dashboard`, `mikrotik`,
`newsaggregator`, `order`, `payment`, `product`, `provisioning`, `radius`, `resource`, dan folder legacy
kosong `landingpage`) masih berupa **scaffold kosong** — lihat [docs/module-map.md](docs/module-map.md)
untuk status lengkap sebelum mengasumsikan salah satunya sudah berjalan.

---

## 🗺️ Roadmap / Belum Diimplementasikan

Bagian ini berisi arah produk yang pernah direncanakan tapi **belum ada di kode saat ini**. Dipertahankan
sebagai catatan arah bisnis, bukan status implementasi — jangan dijadikan acuan requirement teknis tanpa
konfirmasi ulang dengan pemilik produk.

- **CRM (Customer Relationship Management)**: lead & contact management, sales pipeline/kanban,
  interaction log, customer segmentation.
- **Company Profile Page**: manajemen portofolio/layanan/blog/tim sebagai varian produk landing page.
- **Module Membership (Loyalty Program)**: tiering system, poin reward, kupon/voucher digital.
- **Module POS (Point of Sales)**: kasir, manajemen inventori, multi-outlet, integrasi membership.
- **Dynamic Custom Domain & Automatic SSL Provisioning** otomatis via Traefik/Caddy + Let's Encrypt
  (saat ini domain binding & verifikasi sudah ada di modul organization, tapi provisioning SSL otomatis
  penuh **perlu diverifikasi** cakupannya).
- **Event-Driven Architecture dengan Message Broker** (RabbitMQ/Kafka) — saat ini event/notification
  memakai pola outbox internal (`internal/core/notification`), belum memakai message broker eksternal.
- **Multi-Language / i18n** untuk konten tenant.

---

## 🏗️ Struktur Folder Aktual

Struktur berikut adalah hasil pembacaan langsung dari repository (bukan rancangan/rekomendasi) — modular
monolith dengan layering `handler → service → repository` per modul. Detail lengkap tiap folder ada di
[docs/repository-context.md](docs/repository-context.md).

```text
zyad.cloud/
├── api/                         # Spesifikasi API
│   ├── openapi.yaml              # Spec utama (~6968 baris, OpenAPI 3.0.3)
│   └── openapi-landing.yaml      # Draft stub lama (Confirmed: belum digabung) — akan di-merge ke openapi.yaml
│
├── cmd/                         # Entry point aplikasi (4 binary terpisah)
│   ├── api/main.go               # HTTP API server
│   ├── migrate/main.go           # Migration runner (-direction up|down, -dir, -steps)
│   ├── seed/main.go              # Seed runner (-name super-admin | platform-organization)
│   └── worker/main.go            # Notification worker (-once untuk single batch)
│
├── internal/                    # Kode privat aplikasi
│   ├── app/                      # Bootstrap, dependency injection manual, router, server
│   │   ├── app.go                  # App struct, New()/Run(), wiring seluruh dependency
│   │   ├── dependency.go           # struct Dependencies yang dioper ke router
│   │   ├── router.go               # Registrasi route + middleware global (Gin)
│   │   ├── server.go               # Lifecycle HTTP server (graceful shutdown)
│   │   └── module_routes.go        # Registrasi RegisterRoutes() semua modul
│   │
│   ├── config/                   # Config loader custom (env var + .env), TANPA viper/envconfig
│   │   ├── config.go                # Root struct Config
│   │   ├── load.go                  # Fungsi LoadXxx() per section + parsing .env
│   │   └── validate.go              # Validasi config untuk startup
│   │
│   ├── platform/                 # Adapter infrastruktur eksternal
│   │   ├── database/                # Pool pgx, migration runner, tenant transaction, RLS test
│   │   ├── logger/, mail/, metrics/, mikrotik/, redis/, storage/, whatsapp/, xendit/
│   │
│   ├── core/                     # Fondasi lintas modul
│   │   ├── auth/                    # JWT (HMAC-SHA256), Google OAuth verify, password hashing
│   │   ├── cache/                   # Abstraksi cache
│   │   ├── crypto/                  # Token/random string generator
│   │   ├── errors/                  # Error standar + mapping ke HTTP response
│   │   ├── event/                   # Publish/subscribe event internal
│   │   ├── http/                    # Helper response (OK/Error envelope)
│   │   ├── idempotency/             # Pencegah duplicate request (order/payment)
│   │   ├── middleware/              # Authenticate, ResolveOrganization, CORS, RequestID, Recovery
│   │   ├── notification/            # Sistem notifikasi: handler/service/repository/dispatcher/consumer
│   │   ├── permission/              # RBAC custom: handler/service/repository/middleware
│   │   ├── tenant/                  # Tipe Context multi-tenant (OrganizationID, ResolutionSource, dll)
│   │   └── validation/              # Custom Gin validator
│   │
│   ├── modules/                  # Modul domain bisnis — lihat docs/module-map.md untuk status lengkap
│   │   ├── landing/                 # STABIL — landing page builder (modul kanonik)
│   │   ├── landingpage/             # KOSONG (0 file) — sisa scaffold lama, jangan dipakai
│   │   ├── organization/            # STABIL — multi-tenant core
│   │   ├── billing/                 # WIP — plan/subscription/invoice/payment/entitlement
│   │   ├── user/                    # STABIL — auth + user management
│   │   └── account, asset, contract, dashboard, mikrotik, newsaggregator, order, payment,
│   │       product, provisioning, radius, resource   # STUB — scaffold kosong, belum digarap
│   │
│   └── shared/                   # Helper lintas modul
│       ├── pagination/              # Baru berisi doc.go — belum ada helper terpakai
│       ├── response/                # Envelope response sukses/error (Success/Message/Data/Meta)
│       └── utils/                   # Utility string/time/slug/phone
│
├── migrations/                  # 57 pasang file .up/.down.sql (000001–000057)
├── docs/                        # Dokumentasi — lihat docs/repository-context.md untuk index lengkap
├── scripts/, tests/              # Script pendukung & test tambahan
├── .env.example
├── go.mod / go.sum
├── AGENTS.md                    # Instruksi kerja untuk AI coding agent
└── README.md                    # Dokumen ini
```

**Catatan**: tidak ada Dockerfile/docker-compose/Makefile/CI workflow di repo ini — lihat
[docs/development-guide.md](docs/development-guide.md) untuk cara setup lokal.

---

## 📚 Development Documentation

Sebelum mengikuti workflow per fitur di bawah, baca dulu 8 dokumen konteks lintas-modul di `docs/` sebagai
orientasi umum: [repository-context.md](docs/repository-context.md),
[development-guide.md](docs/development-guide.md), [module-map.md](docs/module-map.md),
[api-contract-review.md](docs/api-contract-review.md), [database-context.md](docs/database-context.md),
[permission-context.md](docs/permission-context.md),
[development-traceability.md](docs/development-traceability.md),
[next-development-tasks.md](docs/next-development-tasks.md).

### Multi-Tenant Workflow

Gunakan urutan berikut saat mengerjakan capability multi-tenant atau module yang menyimpan data organization:

1. `README.md` untuk konteks platform.
2. `docs/reference-multi-tenant.md` untuk architecture, ownership, isolation, dan platform organization.
3. `docs/multi-tenant-development-tasks.md` untuk breakdown pekerjaan.
4. `docs/multi-tenant-traceability-index.md` untuk requirement, API, migration, permission, dan dependency.
5. `docs/multi-tenant-schema-query-audit.md` sebelum normalisasi schema/query yang sudah ada.
6. `docs/migration-guide.md` sebelum membuat migration.

Zyad Cloud sendiri direpresentasikan sebagai platform organization. Landing Page marketing platform dan Landing Page customer memakai module yang sama dengan `organization_id` yang konkret.

### Landing Page Workflow

Gunakan urutan berikut saat mengerjakan module Landing Page:

1. `README.md` untuk konteks platform.
2. `docs/reference-multi-tenant.md` untuk tenant context dan isolation contract.
3. `docs/reference-landing-page.md` untuk requirement, boundary, dan kapasitas.
4. `docs/landing-page-development-tasks.md` untuk breakdown pekerjaan.
5. `docs/landing-page-traceability-index.md` untuk traceability requirement, API, migration, permission, event, dan status.
6. `docs/landing-page-public-api-contract.md` untuk kontrak Frontend Vue dan public renderer.
7. `docs/migration-guide.md` sebelum membuat migration.

Target canonical module adalah `internal/modules/landing`.

### Billing Plan Workflow

Gunakan urutan berikut saat mengerjakan Plan, Billing, Subscription, Entitlement, Quota, Invoice, dan Payment:

1. `README.md` untuk konteks platform multi-tenant dan module billing.
2. `AGENTS.md` untuk batas layer, migration, dan gaya kerja repository.
3. `docs/reference-plan-billing-subscribe.md` untuk source requirement awal.
4. `docs/billing-plan-concept-reference.md` untuk konsep domain, boundary, table, service, API, dan integrasi existing.
5. `docs/billing-plan-development-tasks.md` untuk breakdown pekerjaan bertahap.
6. `docs/billing-plan-development-traceability.md` untuk traceability requirement, migration, seed, API, permission, dan test.
7. `docs/migration-guide.md` sebelum membuat migration billing.

Target canonical module adalah `internal/modules/billing`. Runtime entitlement dan usage quota harus selaras dengan `organization_entitlements` dan `organization_usage_counters` existing.

---

## 🧪 Development Commands

### Unit Test

Jalankan test biasa tanpa kebutuhan database:

```bash
go test ./...
```

### Integration Test Database

Integration test database bersifat opt-in dan hanya berjalan dengan build tag `integration`. Pastikan `.env` berisi config test database terpisah, bukan database development/production:

```env
TEST_DB_TYPE=postgres
TEST_DB_HOST=localhost
TEST_DB_PORT=5432
TEST_DB_USERNAME=postgres
TEST_DB_PASSWORD=postgres
TEST_DB_NAME=zyad_cloud_test
TEST_DB_SCHEMA=public
TEST_DB_SSL=disable
TEST_DB_CONNECT_TIMEOUT_SECONDS=5
```

Database test wajib mengandung kata `test` pada `TEST_DB_NAME`. Helper test akan menjalankan migration dan melakukan cleanup table notification dengan `TRUNCATE ... CASCADE`.

Jalankan semua integration test notification repository:

```bash
go test -tags=integration ./internal/core/notification/repository
```

Untuk compile-check integration test tanpa menjalankan test yang menyentuh database:

```bash
go test -tags=integration -run '^$' ./internal/core/notification/repository
```

### Notification Worker

Jalankan satu batch outbox dan retry notification:

```bash
go run ./cmd/worker -once
```

Jalankan worker loop:

```bash
go run ./cmd/worker
```

Worker memakai env `NOTIFICATION_WORKER_INTERVAL_SECONDS` dan `NOTIFICATION_WORKER_BATCH_SIZE`. Default worker hanya mendaftarkan email dispatcher; WhatsApp tidak diproses worker sampai provider WhatsApp tersedia. Email akan memakai SMTP saat `MAIL_HOST` terisi, dan otomatis fallback ke `noop` saat `MAIL_HOST` kosong untuk local test.

---

## 💡 Ide Tambahan & Peningkatan

Ide/rekomendasi peningkatan platform (SSL otomatis, message broker, payment gateway, feature flagging per
paket langganan, i18n) sudah digabung ke section [🗺️ Roadmap / Belum Diimplementasikan](#️-roadmap--belum-diimplementasikan)
di atas bersama item roadmap lain, supaya semua hal yang "belum ada di kode" ada di satu tempat.
**Payment Gateway Integration** khususnya sudah **sebagian berjalan** (Xendit, lewat modul `billing` —
lihat [docs/billing-plan-concept-reference.md](docs/billing-plan-concept-reference.md)), bukan lagi murni ide.

## 📄 Lisensi
Hak Cipta © 2026. Seluruh hak cipta dilindungi undang-undang.
