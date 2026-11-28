# Agent Working Guide — zyad.cloud Backend

## Bahasa dan gaya kerja
- Jelaskan hasil kerja dalam Bahasa Indonesia.
- Gunakan Bahasa Inggris untuk code, command, file name, branch name, dan commit message.
- Buat perubahan kecil dan bertahap.
- Jangan menambah dependency production tanpa konfirmasi.
- Jangan mengubah arsitektur utama tanpa konfirmasi.
- Jalankan `git config core.hooksPath .githooks` sekali di awal (kalau belum) — repo ini punya pre-commit
  hook yang menolak commit berisi file scratch/binary sekali pakai (`bin/`, `*_bin`, `fix*.py`,
  `scratch*.go`, `test_resolve.go`). Jangan pernah commit file semacam itu meskipun hook belum aktif.
- **Repo ini publik.** Jangan commit runbook/skrip operasional produksi yang berisi data pribadi, IP/host
  infrastruktur, credential, dump, atau data tenant nyata. Simpan itu di luar repo.

## Scope repo
- Bacalah dokumen `README.md` terlebih dahulu untuk memahami konteks dari repo.
- Dokumentasi development berada di `docs/`.
- Untuk gambaran lintas-modul (stack, arsitektur, module map, database, permission, traceability, task
  lanjutan), baca dulu 8 dokumen konteks: `docs/repository-context.md`, `docs/development-guide.md`,
  `docs/module-map.md`, `docs/api-contract-review.md`, `docs/database-context.md`,
  `docs/permission-context.md`, `docs/development-traceability.md`, `docs/next-development-tasks.md`.
- Frontend (Vue 3) berada di repo terpisah (`../frontend`). Perubahan kontrak API wajib diselaraskan ke sana.

## Stack (aktual, lihat `go.mod`)
- **Go 1.26**, module `zyad.cloud`.
- **Gin** untuk HTTP routing/middleware.
- **PostgreSQL** via `pgx/v5` (SQL eksplisit, tanpa ORM). Runner migration custom di `cmd/migrate`.
- **Redis** (`go-redis/v9`) — saat ini **hanya untuk session**. Jangan perluas tanggung jawabnya ke cache
  umum/queue tanpa keputusan eksplisit.
- Object storage: driver lokal atau S3-compatible (MinIO) via `aws-sdk-go-v2` (`internal/platform/storage`).
- Payment gateway: **DOKU** (`internal/platform/doku`, webhook `/api/v1/webhooks/doku` di modul
  `receivable`). Xendit sudah tidak dipakai.
- WhatsApp: WAHA self-hosted (`internal/platform/whatsapp`, modul `whatsapp`).
- MikroTik: REST v7 + binary API v6 (`internal/platform/mikrotik`, modul `mikrotik`).
- Sanitasi HTML landing: `bluemonday`.

## Struktur folder (aktual)

```text
cmd/
├── api/               # HTTP server
├── worker/            # worker notification/outbox, receivable, mikrotik, mailbox sync, WhatsApp reconciler (-once untuk satu batch)
├── migrate/           # runner migration (-direction up|down -dir migrations -steps N)
├── seed/              # -name super-admin | platform-organization | self-serve
├── entitlementgrant/  # grant entitlement platform_override (lihat docs/runbook-entitlement-grant.md)
├── accessmigrate/     # migrasi akses R4-S5 (pensiun plan/subscription/billing lama)
└── financesmoke/      # smoke test modul finance
api/
├── openapi.yaml           # kontrak API utama — jaga sinkron dengan implementasi
└── openapi-landing.yaml
internal/
├── app/        # composition root: dependency wiring, router, module routes, dan ADAPTER lintas modul
│               # (mis. mikrotik_*.go, receivable.go, self_serve.go, quota_meters.go)
├── config/     # load + validate env
├── core/       # auth, businesstime, cache, crypto, errors, event, http, idempotency, middleware,
│               # notification, permission, tenant, validation
├── platform/   # database, doku, logger, mail, metrics, mikrotik, redis, storage, whatsapp
├── modules/    # lihat tabel di bawah
└── shared/     # docpdf, pagination, phone, pricing, publiclink, response, utils
migrations/     # 000001 … 000186+ (pasangan .up.sql/.down.sql)
scripts/        # db/, sql/ (helper; jangan taruh skrip ops produksi di sini)
deploy/         # deploy.sh + ecosystem.config.js (pm2)
docs/
```

### Status modul `internal/modules/`

| Status | Modul |
|---|---|
| Aktif | `user`, `organization`, `landing`, `crm`, `catalog`, `receivable`, `mikrotik`, `whatsapp`, `mailbox`, `finance`, `asset` (storage tenant + kuota), `product` (hanya registry `product_features`), `social` (identitas warga: klaim relasi pelanggan ke kontak CRM + atribusi referral; `docs/social-identity.md`) |
| Stub (scaffold kosong) | `account`, `contract`, `dashboard`, `newsaggregator`, `order`, `payment`, `provisioning`, `radius`, `resource` |
| Dihapus (R4-S5, migration 000154) | `billing`, `subscription` — digantikan `receivable` (invoice/DOKU), contract + `organization_entitlements` |

Status detail dan risiko tiap modul: `docs/module-map.md`. Cek isi folder aktual sebelum percaya dokumen.

## Development pattern
- Setiap module berada di `internal/modules/<module_name>`.
- Layer standar: `handler`, `service`, `repository`, `dto`, ditambah `model` **atau** `domain`
  (modul lama seperti `user`/`organization` memakai `model/`; modul baru seperti `crm`, `landing`,
  `receivable`, `mikrotik`, `whatsapp`, `mailbox`, `catalog` memakai `domain/`). Ikuti konvensi modul
  yang sedang diubah; jangan mencampur keduanya dalam satu modul.
- Handler hanya menangani HTTP binding, auth context, validasi request, dan response.
- Service memegang business rule, transaction boundary, idempotency, dan integrasi antar module.
- Repository hanya mengakses database dan tidak boleh berisi business rule.
- Modul **tidak saling import** langsung untuk integrasi lintas modul; definisikan interface kecil di modul
  konsumen dan implementasikan adapter-nya di `internal/app` (pola yang sudah dipakai receivable,
  mikrotik, self-serve).
- Platform integration ditempatkan di `internal/platform`.
- Shared core seperti auth, middleware, validation, response, permission, dan crypto ditempatkan di `internal/core`.
- Entitlement/kuota tenant: source of truth `organization_entitlements` dan `organization_usage_counters`
  (guard di `organization/service`). Jangan membuat source of truth kedua tanpa migration deprecation eksplisit.
- Endpoint baru/berubah wajib disinkronkan ke `api/openapi.yaml`; jangan mendokumentasikan perilaku yang
  tidak diimplementasikan.

## Database
- Schema database langsung memakai `public`.
- Cleanup schema harus dicatat di migration terpisah agar traceable.
- Hindari TypeORM-style auto sync; semua perubahan database harus melalui migration.
- Setiap migration wajib punya pasangan `up` dan `down`, nomor berurutan setelah migration terakhir di
  `migrations/` (cek nomor terbaru di branch target sebelum membuat file — branch paralel sering bentrok
  nomor). Panduan: `docs/migration-guide.md`.
- Tabel tenant baru wajib memanggil `apply_organization_rls('<table>'::regclass)` dan tetap memfilter
  `organization_id` eksplisit di query. Detail: `.claude/rules/rls.md`, `docs/multi-tenant-rls.md`.
- Timestamp: sesi DB dikunci UTC, nilai dari Go wajib `.UTC()`, batas "hari" bisnis (Asia/Jakarta)
  dihitung di Go lewat `internal/core/businesstime`. Detail: `docs/timezone-handling.md`.
- Permission baru diseed lewat migration (format `resource.action`), lihat `docs/permission-context.md`.

## Commands

```bash
go mod download
go build ./... && go vet ./... && go test ./...          # sama dengan CI (.github/workflows/ci.yml)

# Integration test: build tag `integration`, butuh DB test terpisah (TEST_DB_*, nama DB wajib mengandung "test")
go test -tags=integration ./internal/modules/<module>/repository/...
go test -tags=integration -run '^$' ./...                 # compile-check saja, tanpa menyentuh DB

go run ./cmd/migrate -direction up -dir migrations -steps 0
go run ./cmd/seed -name super-admin                       # wajib sebelum platform-organization
go run ./cmd/seed -name platform-organization
go run ./cmd/api
go run ./cmd/worker -once                                 # satu batch, atau tanpa -once untuk loop
```

Detail env dan setup lokal: `docs/development-guide.md` dan README bagian "Development Commands".

## Branch & rilis
- Kerja di branch `feat/<nama>` / `fix/<nama>` dari `staging-dev` terbaru.
- `staging-dev` = branch integrasi untuk server DEV; masuk lewat **PR**, bukan push langsung.
- `main` = produksi; hanya menerima PR `staging-dev → main` saat rilis, lalu diberi tag SemVer
  `vX.Y.Z` (tag yang sama di repo frontend). Produksi hanya dideploy dari tag.
- `.github/workflows/deploy.yml` (push `main`) saat ini **tidak dipakai** untuk deploy; jangan
  mengandalkannya. CI (`ci.yml`) tetap berjalan di PR.
- Commit/push/PR/deploy hanya saat diminta.

## Workflow per area

Reading-order checklist juga tersedia sebagai skill di `.claude/skills/*-workflow/`.

### Auth dan User
1. `README.md` untuk konteks repo.
2. `docs/reference-auth-user.md` untuk source requirement.
3. `docs/auth-user-development-tasks.md` untuk breakdown pekerjaan.
4. `docs/auth-user-traceability-index.md` untuk melacak kaitan requirement, API, migration, dan seed.
5. `docs/auth-user-public-api-contract.md` untuk kontrak response dan endpoint publik.
6. `docs/auth-user-migration-seed-plan.md` untuk urutan migration dan seed env.
7. `docs/google-auth-*.md` dan `docs/mailbox-microsoft-oauth.md` untuk OAuth Google/Microsoft.

### Notification
1. `README.md` untuk konteks repo.
2. `docs/reference-notification.md` untuk source requirement.
3. `docs/notification-development-tasks.md` untuk breakdown pekerjaan.
4. `docs/notification-traceability-index.md` untuk melacak requirement, config, API, migration, dan status.
5. `docs/migration-guide.md` sebelum membuat migration notification.

### Multi-tenant
`docs/reference-multi-tenant.md` → `docs/multi-tenant-development-tasks.md` →
`docs/multi-tenant-traceability-index.md` → `docs/multi-tenant-rls.md` → `docs/multi-tenant-schema-query-audit.md`.

### Landing Page
`docs/reference-landing-page.md` → `docs/landing-page-builder.md` (GrapesJS) →
`docs/landing-page-development-tasks.md` → `docs/landing-page-traceability-index.md` →
`docs/landing-page-public-api-contract.md`.

### Produk, Entitlement, Invoice, dan Payment
Modul `billing`/`subscription` sudah dihapus; baca dokumen `billing-plan-*` hanya sebagai konteks historis.
1. `docs/product-subscription-billing-concept.md` dan `docs/product-subscription-billing-traceability.md`.
2. `docs/reference-catalog.md` (produk/harga tenant) dan `docs/reference-receivable.md` (invoice + DOKU).
3. `docs/runbook-doku-notification.md`, `docs/runbook-entitlement-grant.md`, `docs/free-plan-rollout.md`,
   `docs/tenant-products-rollout.md`.
4. `docs/migration-guide.md` sebelum membuat migration.

### CRM, WhatsApp, MikroTik
- CRM: `docs/reference-crm.md`.
- WhatsApp: `docs/reference-whatsapp.md`, `docs/whatsapp-development-tasks.md`, `docs/whatsapp-traceability-index.md`.
- MikroTik/isolir: `docs/reference-mikrotik.md`, `docs/runbook-mikrotik-isolir-setup.md`,
  `docs/checklist-uji-lab-isolir-publik.md`. Router uji hanya router lab/dev; jangan menjalankan setup
  isolir ke router produksi tanpa izin eksplisit.
