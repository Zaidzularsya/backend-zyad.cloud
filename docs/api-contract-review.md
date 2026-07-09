# API Contract Review — OpenAPI vs Kode Aktual

> Metodologi: perbandingan dilakukan dengan (1) ekstraksi semua path dari `api/openapi.yaml` (110 path)
> via pencocokan pola YAML, dan (2) ekstraksi literal route registration (`router.GET/POST/PATCH/PUT/DELETE`)
> dari seluruh `internal/modules/*/handler/*.go` dan `internal/core/*/handler/*.go`. Karena banyak handler
> mendaftarkan route relatif di dalam route group (prefix ditentukan di file lain seperti `routes.go`/
> `module_routes.go`/`router.go`), perbandingan ini **bukan diff baris-per-baris otomatis** — temuan di
> bawah adalah hasil cross-check manual pada modul yang polanya paling jelas. Bagian yang belum bisa
> dipastikan ditandai status **Needs code verification**; bagian yang sudah ada keputusan manual dari
> pemilik repo ditandai **Confirmed**/**Deferred**/**Future improvement** sesuai konteksnya (lihat tabel
> Ringkasan Verifikasi Manual di [repository-context.md](repository-context.md) bagian 10).

## 1. Cakupan OpenAPI Saat Ini

`api/openapi.yaml` (OpenAPI 3.0.3, ~6968 baris) mendokumentasikan **110 path**, mengelompok ke:
- Auth (`/api/v1/auth/*`)
- Users (`/api/v1/users/me*`, `/api/v1/admin/users/*`)
- Organizations (`/api/v1/organization*`, `/api/v1/platform/organizations*`)
- Landing Pages admin (`/api/v1/admin/landing-pages*`, `/api/v1/admin/landing/*`, `/api/v1/admin/landing-submissions*`)
- Landing public (`/api/v1/public/landing/*`)
- Product/Catalog platform (`/api/v1/platform/product/*` — plans/prices/features/entitlements, hasil rename
  dari `/api/v1/platform/billing/plans*`/`features*` per refactor domain-split 2026-07-01)
- Subscription platform (`/api/v1/platform/subscriptions*` — hasil rename dari
  `/api/v1/platform/billing/subscriptions*`)
- Billing platform, ramping (`/api/v1/platform/billing/invoices*` — TIDAK berubah)
- Billing tenant (`/api/v1/app/billing/*` — TIDAK berubah)
- Audit (`/api/v1/admin/login-histories`, `/api/v1/admin/audit-logs`)

### Status `api/openapi-landing.yaml` (Confirmed decision, 2026-07-01)

`api/openapi-landing.yaml` (22 baris) dibaca langsung isinya: file ini adalah **draft stub lama**, bukan
spec paralel yang lengkap. Baris komentar di dalamnya secara eksplisit berbunyi *"This file is a stub
mapping of the landing page endpoints for openapi.yaml — Copy these under the paths: node in
api/openapi.yaml"*, dan hanya berisi 2 path (`/api/v1/admin/landing-pages`,
`/api/v1/admin/landing-pages/{id}`) dengan summary minimal — keduanya **sudah terdokumentasi lengkap** di
`api/openapi.yaml`.

**Keputusan resmi**: *"api/openapi.yaml is the canonical OpenAPI source. Landing OpenAPI definitions
should be merged into api/openapi.yaml."* Karena isi stub ini sudah sepenuhnya tercakup di
`api/openapi.yaml`, tidak ada konten baru yang perlu di-merge — tindak lanjutnya adalah **membersihkan/
mengarsipkan file stub ini** (bukan menyalin ulang isinya) agar tidak ada dua sumber yang membingungkan.
Jika ke depan tim tetap butuh pemisahan modular untuk file OpenAPI, boleh dipertahankan sebagai file
internal, tapi bundle resmi/final tetap wajib `api/openapi.yaml`. Task konkret ada di
[next-development-tasks.md](next-development-tasks.md).

## 2. Mismatch yang Ditemukan (Terverifikasi)

| Endpoint di kode | Ditemukan di | Status di OpenAPI | Rekomendasi |
|---|---|---|---|
| `GET/POST/PATCH/DELETE /admin/permissions`, `/admin/permissions/:id`, `/admin/permissions/grouped` | `internal/core/permission/handler/*.go` | **Tidak terdokumentasi** — tidak ada path `admin/permissions*` di `openapi.yaml` | Tambahkan section Permission Management ke OpenAPI |
| `GET/POST/PATCH/DELETE /admin/roles`, `/admin/roles/:id`, `/admin/roles/:id/permissions*` | `internal/core/permission/handler/*.go` | **Tidak terdokumentasi** | Tambahkan section Role Management ke OpenAPI |
| `GET /admin/permission-matrix` | `internal/core/permission/handler/*.go` | **Tidak terdokumentasi** | Tambahkan ke OpenAPI, termasuk contoh response matrix |
| `GET/POST/PATCH/DELETE /admin/users/:id/permissions*`, `/admin/users/:id/roles*` | `internal/modules/user/handler/*.go` atau `internal/core/permission/handler/*.go` — **Needs code verification** untuk lokasi pasti | **Tidak terdokumentasi** | Tambahkan ke OpenAPI |
| `POST /onboarding/workspace` | `internal/modules/organization/handler/onboarding_handler.go` (nama file indikatif) — **Needs code verification** untuk path file pasti | **Tidak terdokumentasi** | Tambahkan ke OpenAPI atau konfirmasi apakah endpoint ini masih dipakai |

Endpoint product/subscription/billing (`/api/v1/platform/product/*`, `/api/v1/platform/subscriptions*`,
`/api/v1/platform/billing/*`, `/api/v1/app/billing/*`) dan landing public
(`resolve`, `preview/{token}`, `forms/{formKey}/submissions`, `forms/{formKey}/uploads`, `events`) sudah
**cocok** antara path OpenAPI dan literal route di handler — tidak ada mismatch signifikan yang ditemukan
di kelompok ini pada level path (belum dicek detail request/response body per endpoint).

## 3. Yang Belum Diverifikasi (Perlu Pengecekan Lanjutan)

Karena volume endpoint besar (110+ path OpenAPI vs 143+ literal route registration di kode), berikut yang
**belum** di-cross-check detail dan perlu dilakukan sebagai task terpisah (lihat
[next-development-tasks.md](next-development-tasks.md)):
- Kecocokan **request body** (field wajib/opsional, tipe) per endpoint terhadap DTO Go aktual
  (`internal/modules/*/dto/request.go`).
- Kecocokan **response body** terhadap DTO Go aktual (`dto/response.go`) — termasuk field yang mungkin
  sudah dihapus/ditambah di kode tapi belum diupdate di spec.
- Kecocokan **query parameter** (filter, sort) — landing module contoh: `PageListFilter` (Status, PageType,
  IsTemplate, IncludeDeleted, Limit, Offset) di `internal/modules/landing/repository/page_contract.go` —
  perlu dicek apakah semua field ini punya representasi query param di OpenAPI.
- Kecocokan **kode error** (setelah refactor domain-split, tersebar di 3 file:
  `internal/modules/product/errors.go` (mis. `PLAN_NOT_FOUND`, `FEATURE_NOT_FOUND`),
  `internal/modules/subscription/errors.go` (mis. `SUBSCRIPTION_NOT_FOUND`, `QUOTA_EXCEEDED`),
  `internal/modules/billing/errors.go` (mis. `PAYMENT_ALREADY_PROCESSED`) — perlu dicek apakah semua kode
  ini terdaftar di skema error response OpenAPI).
- Permission requirement per endpoint (slug seperti `landing.page.read`) — apakah OpenAPI
  mendokumentasikan permission yang dibutuhkan (via `security`/`x-permission` extension atau deskripsi).

## 4. Standar Response Format (Terverifikasi dari Kode)

Sumber: `internal/shared/response/response.go`.

**Success envelope:**
```json
{
  "success": true,
  "message": "optional message",
  "data": { },
  "meta": { }
}
```

**Error envelope:**
```json
{
  "success": false,
  "code": "SOME_ERROR_CODE",
  "message": "human readable message"
}
```

Helper: `response.JSON(c, status, message, data, meta)` dan `response.Error(c, status, code, message)`.
`Success` otomatis `true` jika `status < 400`.

## 5. Pagination — Temuan Penting

`internal/shared/pagination/doc.go` **hanya berisi doc comment**, belum ada implementasi helper pagination
bersama. Setiap modul mengimplementasikan pagination secara ad-hoc di level filter struct repository
masing-masing, contoh: `PageListFilter` di landing module memakai field `Limit`/`Offset` langsung (bukan
`Page`/`PerPage`). **Ini bukan bug**, tapi berarti:
- Tidak ada kontrak pagination response yang seragam terjamin lewat kode bersama — konsistensi response
  `meta` (total, limit, offset) bergantung pada disiplin masing-masing handler.
- Saat menulis/memverifikasi OpenAPI untuk endpoint list, jangan asumsikan field pagination sama di semua
  endpoint — cek DTO response masing-masing modul.
- **Rekomendasi**: jika pola ini mau distandarkan, implementasikan helper di `internal/shared/pagination`
  dan migrasikan modul existing secara bertahap (lihat [next-development-tasks.md](next-development-tasks.md), P2).

## 6. Auth & Permission Requirement pada Endpoint

- Endpoint di bawah `protected` group (`internal/app/router.go`) melewati `middleware.Authenticate()` +
  `middleware.ResolveAuthenticatedOrganization()` sebelum masuk handler.
- Endpoint publik landing (`/api/v1/public/landing/*`) melewati `middleware.ResolvePublicOrganization()` +
  `middleware.RequireActiveTenant()`.
- Permission spesifik dicek di level route registration masing-masing modul via
  `permissionmiddleware.Require`/`RequireOrganization`/`RequireOrganizationOrGlobal` dengan slug seperti
  `landing.page.create`, `organization.billing.manage` (detail lengkap di
  [permission-context.md](permission-context.md)).
- OpenAPI **tidak mendokumentasikan permission slug per endpoint** secara eksplisit — rekomendasi tambahkan
  sebagai extension field (`x-required-permission`) atau minimal di deskripsi endpoint, supaya konsisten
  dengan implementasi.

## 7. Rekomendasi Koreksi OpenAPI (Ringkasan Prioritas)

| Prioritas | Rekomendasi |
|---|---|
| P1 | Tambahkan dokumentasi untuk endpoint Permission/Role management (`admin/permissions*`, `admin/roles*`, `admin/permission-matrix`, `admin/users/:id/permissions*`, `admin/users/:id/roles*`) |
| P1 | Tambahkan/konfirmasi endpoint `onboarding/workspace` |
| P2 | Audit sistematis request/response body per endpoint vs DTO Go (bisa dipecah per modul: user, organization, landing, billing) |
| P2 | Tambahkan anotasi permission requirement (`x-required-permission`) per endpoint |
| P1 | **(Confirmed decision)** Bersihkan/arsipkan `api/openapi-landing.yaml` — isinya sudah sepenuhnya tercakup di `api/openapi.yaml`, jadikan `api/openapi.yaml` satu-satunya source of truth resmi |
