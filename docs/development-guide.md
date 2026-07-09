# Development Guide — zyad.cloud Backend

> Panduan setup lokal dan command development sehari-hari, berdasarkan struktur `cmd/`, `.env.example`, dan
> `internal/config` yang ada di repository saat ini. Lihat [repository-context.md](repository-context.md)
> untuk gambaran arsitektur, dan [database-context.md](database-context.md) untuk detail schema.

## 1. Prasyarat

| Kebutuhan | Catatan |
|---|---|
| Go | versi sesuai `go.mod`: **Go 1.26** |
| PostgreSQL | database utama, wajib jalan sebelum migration/seed/server |
| Redis | **Confirmed (2026-07-01): currently used for session storage only.** Wajib jalan sebelum server start. Pemakaian sebagai cache umum/queue/rate-limit adalah *future improvement*, belum diimplementasikan — jangan asumsikan `internal/core/cache` sudah didukung Redis penuh tanpa cek ulang kode saat fitur itu mulai digarap |
| — | **Tidak ada Dockerfile/docker-compose/Makefile/CI di repo ini.** Semua service (Postgres, Redis) harus disiapkan manual di mesin lokal atau lewat tooling di luar repo ini. Jangan mencari `docker-compose up` — tidak ada. |

## 2. Instalasi Dependency

```bash
go mod download
```

## 3. Environment Variable

Salin `.env.example` menjadi `.env` di root repo, lalu isi minimal variabel berikut agar server bisa jalan:

### Wajib diisi
| Variabel | Keterangan |
|---|---|
| `APP_NAME`, `APP_HOST`, `APP_PORT`, `APP_URL` | identitas & bind address server |
| `APP_FRONTEND_URL` | dipakai untuk CORS ke frontend (`frontend.zyad.cloud`) |
| `APP_SECRET` | secret aplikasi umum |
| `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_NAME`, `DB_TYPE=postgres`, `DB_SCHEMA=public`, `DB_SSL` | koneksi database utama |
| `JWT_SECRET`, `JWT_REFRESH_SECRET` | signing key access & refresh token |
| `PLATFORM_ORGANIZATION_SLUG`, `PLATFORM_ORGANIZATION_NAME`, `PLATFORM_PRIMARY_DOMAIN` | wajib untuk seed `platform-organization` |
| `SEED_ADMIN_NAME`, `SEED_ADMIN_USERNAME`, `SEED_ADMIN_EMAIL`, `SEED_ADMIN_PASSWORD` | wajib untuk seed `super-admin` |
| `REDIS_HOST`, `REDIS_PORT` | koneksi Redis (default `localhost:6379`) |

### Opsional (punya default atau fitur opsional)
| Variabel | Default/Perilaku jika kosong |
|---|---|
| `NODE_ENV` | tanpa default eksplisit — cek `internal/config` untuk perilaku saat kosong |
| `AUTH_GOOGLE_ENABLED` | `false` — login Google nonaktif jika tidak di-set |
| `MAIL_HOST` | jika kosong, email fallback ke provider `noop` (tidak benar-benar mengirim) |
| `WHATSAPP_PROVIDER`, `DISCORD_PROVIDER` | default `noop` |
| `XENDIT_BASE_URL`, `XENDIT_API_KEY`, `XENDIT_WEBHOOK_TOKEN` | payment gateway — tanpa ini, flow pembayaran billing tidak akan berfungsi (modul `billing` masih WIP terkait ini, lihat [next-development-tasks.md](next-development-tasks.md)) |
| `TEST_DB_*` | konfigurasi database khusus test (lihat bagian Testing) |

Daftar lengkap variabel ada di `.env.example` (root repo) — dokumen ini hanya menyoroti yang penting untuk
mulai development. Untuk detail default masing-masing (mis. `JWT_EXPIRES_IN`, `AUTH_OTP_MAX_ATTEMPTS`),
baca langsung `internal/config/load.go` karena default sebagian di-hardcode di kode, bukan di `.env.example`.

## 4. Database Setup

### 4.1. Buat database

Buat database PostgreSQL kosong sesuai `DB_NAME` di `.env` (manual via `psql`/tool DB — tidak ada script
otomatis di repo ini).

### 4.2. Jalankan migration

```bash
go run ./cmd/migrate -direction up -dir migrations -steps 0
```

- `-direction up|down`
- `-dir migrations` (folder default)
- `-steps 0` artinya jalankan semua migration pending; isi angka untuk membatasi jumlah step.
- Migration bernomor `000001`–`000057`, harus dijalankan berurutan (runner custom ini menangani version
  tracking — lihat [database-context.md](database-context.md) untuk daftar lengkap).

### 4.3. Jalankan seed

**Urutan wajib** (terverifikasi dari kode `internal/modules/organization/seeder`, fungsi
`SeedPlatformOrganization` memanggil `findPlatformOwner()` yang mencari user super-admin yang sudah ada):

```bash
# 1. Seed super-admin dulu — WAJIB pertama
go run ./cmd/seed -name super-admin

# 2. Baru seed platform organization
go run ./cmd/seed -name platform-organization
```

Menjalankan `platform-organization` sebelum `super-admin` akan gagal karena `findPlatformOwner` tidak akan
menemukan user pemilik.

## 5. Menjalankan Aplikasi

```bash
# HTTP API server
go run ./cmd/api

# Notification worker — loop kontinu
go run ./cmd/worker

# Notification worker — satu batch lalu keluar
go run ./cmd/worker -once
```

Worker memproses tabel `notification_outbox_events` (pola outbox) dan mengirim lewat dispatcher yang
terdaftar (email selalu ada; WhatsApp/Discord tergantung provider yang dikonfigurasi — default `noop`).
Endpoint health check tersedia di `GET /healthz`, `GET /readyz`, `GET /api/v1/meta`.

## 6. Testing

### 6.1. Unit test (tanpa database)

```bash
go test ./...
```

### 6.2. Integration test (butuh database terpisah)

Integration test bersifat opt-in via build tag `integration`. **Wajib** memakai database test terpisah
(bukan database development) — helper test akan menjalankan migration lalu `TRUNCATE ... CASCADE` pada
sebagian tabel, jadi jangan arahkan ke database yang datanya penting.

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

`TEST_DB_NAME` **wajib mengandung kata "test"** (safety check di helper test agar tidak salah target).

```bash
# Jalankan semua integration test (contoh: notification repository)
go test -tags=integration ./internal/core/notification/repository

# Compile-check saja tanpa menjalankan test yang menyentuh DB
go test -tags=integration -run '^$' ./internal/core/notification/repository

# Jalankan integration test modul lain — ganti path sesuai modul
go test -tags=integration ./internal/modules/organization/...
go test -tags=integration ./internal/modules/landing/...
```

## 7. Build

```bash
go build ./...
```

Ada binary hasil build sebelumnya di root (`api_bin`) dan folder `bin/` — **status: Needs code verification**
(belum ada keputusan manual) apakah ini artefak deploy manual atau sisa build lokal developer sebelumnya;
jangan asumsikan ada pipeline build otomatis karena tidak ditemukan CI config.

## 8. Lint / Format

- Format standar Go: `gofmt -l .` atau `go fmt ./...`
- **golangci-lint (Confirmed decision, config belum ada — status: "intended to be used, but in-repo
  configuration needs verification or creation")**: proyek ini akan memakai `golangci-lint` sebagai standar
  quality gate. Dicek ulang pada 2026-07-01 — tidak ditemukan file config apapun di root repo:
  `.golangci.yml`, `.golangci.yaml`, `.golangci.toml`, maupun `.golangci.json` (juga tidak ada `Makefile`).
  **Jangan menulis seolah-olah config sudah ada** sampai file konfigurasinya benar-benar dibuat dan di-commit.
  Sampai config dibuat, jalankan minimal:

```bash
go vet ./...
gofmt -l .
```

  Rekomendasi config minimal `.golangci.yml` untuk memulai (sesuaikan linter set dengan kebutuhan tim saat
  dibuat — lihat task follow-up di [next-development-tasks.md](next-development-tasks.md)):

```yaml
run:
  timeout: 5m
linters:
  enable:
    - govet
    - staticcheck
    - unused
    - errcheck
```

## 9. Coding Convention (ringkasan dari AGENTS.md)

- Setiap modul di `internal/modules/<nama>` memakai layer `handler → service → repository → dto/model`.
- Handler hanya menangani HTTP binding, auth context, validasi request, response — tidak boleh berisi
  business rule.
- Service memegang business rule, transaction boundary, idempotency, integrasi antar modul.
- Repository hanya akses database, tidak boleh berisi business rule.
- Platform integration (eksternal) di `internal/platform`. Shared core (auth, middleware, validation,
  response, permission, crypto) di `internal/core`.
- Semua perubahan schema database **wajib** lewat migration (`migrations/`), tidak ada auto-sync.
- Perubahan kecil dan bertahap; jangan menambah dependency production tanpa konfirmasi; jangan mengubah
  arsitektur utama tanpa konfirmasi (lihat `AGENTS.md` lengkap).

## 10. Workflow Development yang Disarankan

1. Baca [repository-context.md](repository-context.md) untuk orientasi umum.
2. Cek [module-map.md](module-map.md) untuk memastikan modul yang akan dikerjakan statusnya stabil/WIP/stub.
3. Ikuti "workflow" per fitur di `AGENTS.md` (Auth/User, Notification, Billing Plan) atau `README.md`
   (Multi-Tenant, Landing Page) untuk urutan baca dokumen spesifik fitur.
4. Sebelum membuat migration baru, baca `docs/migration-guide.md` dan
   [database-context.md](database-context.md) (terutama catatan RLS).
5. Sebelum menambah/mengubah endpoint, cek [api-contract-review.md](api-contract-review.md) untuk standar
   response format dan mismatch yang sudah diketahui.
6. Tulis unit test minimal untuk service baru; tambahkan integration test bila menyentuh query/repository.

## 11. Common Issues

| Gejala | Kemungkinan Penyebab | Solusi |
|---|---|---|
| `platform-organization` seed gagal, error "owner not found" | Seed `super-admin` belum dijalankan | Jalankan `go run ./cmd/seed -name super-admin` terlebih dahulu |
| Integration test gagal konek DB | `TEST_DB_*` belum diisi atau mengarah ke DB yang salah | Cek `.env`, pastikan `TEST_DB_NAME` mengandung "test" |
| Email tidak terkirim saat development | `MAIL_HOST` kosong → fallback `noop` (memang disengaja) | Isi `MAIL_HOST`/`MAIL_*` jika ingin email benar-benar terkirim |
| WhatsApp/Discord notifikasi tidak jalan | Provider default `noop` | Set `WHATSAPP_PROVIDER`/`DISCORD_PROVIDER` sesuai kredensial yang tersedia |
| Query RLS mengembalikan 0 baris di tabel `landing_*` | Session variable `app.organization_id` belum di-set pada koneksi/transaksi | Pastikan alur request melewati `middleware.ResolveAuthenticatedOrganization`/`ResolvePublicOrganization` sebelum query, lihat `internal/platform/database/tenant_transaction.go` |
| Bingung mencari `docker-compose.yml` | Repo ini memang tidak punya Docker setup | Jalankan Postgres/Redis secara manual atau via tooling infra terpisah (di luar repo) |
