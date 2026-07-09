# Next Development Tasks — zyad.cloud Backend

> Daftar task lanjutan berdasarkan kondisi nyata repository (per 2026-07-01), disusun dari temuan riset di
> [repository-context.md](repository-context.md), [database-context.md](database-context.md),
> [permission-context.md](permission-context.md), dan [api-contract-review.md](api-contract-review.md).
> Task ini melengkapi (bukan menggantikan) daftar task yang sudah ada di `docs/*-development-tasks.md`
> per fitur.

## P0 — Kritis (Risiko Data/Keamanan atau Blocking)

### P0-1. Klarifikasi & tindak lanjut cakupan RLS untuk `organization_*`, `customer_subscriptions`, dan `billing_*`
- **Alasan prioritas**: RLS hanya melindungi tabel `landing_*`. Tabel `organization_*`,
  `customer_subscriptions`/`subscription_events`, dan `billing_*` menyimpan data sensitif (membership,
  entitlement, subscription, invoice, payment) tanpa lapisan pertahanan database — bug filter manual di
  satu query bisa membocorkan data lintas tenant.
- **File yang perlu disentuh**: audit seluruh `internal/modules/organization/repository/*.go`,
  `internal/modules/subscription/repository/*.go`, dan `internal/modules/billing/repository/*.go` (setelah
  refactor domain-split 2026-07-01 — sebelumnya semua di `internal/modules/billing/repository/*.go`) untuk
  memastikan setiap query men-filter `organization_id`; jika diputuskan perlu RLS, tambahkan migration baru
  mengikuti pola `apply_organization_rls()` dari `migrations/000019_add_rls_foundation.up.sql`.
- **Risiko**: migration RLS baru bisa mematahkan query existing yang belum set `app.organization_id` di
  transaksi (mis. job worker/seeder) — perlu test integration menyeluruh sebelum deploy.
- **Acceptance criteria**: keputusan didokumentasikan eksplisit (RLS ditambahkan ATAU alasan cukup dengan
  filter manual + checklist code review); jika RLS ditambahkan, semua integration test terkait
  organization/subscription/billing tetap hijau.
- **Test yang perlu dibuat**: test yang mencoba query lintas-tenant tanpa filter eksplisit dan
  mengharapkan hasil kosong (mirip `rls_integration_test.go` yang sudah ada untuk landing).

### P0-2. Audit query manual organization/subscription/billing untuk kebocoran filter tenant
- **Alasan prioritas**: mitigasi cepat sebelum P0-1 selesai (RLS butuh waktu lebih lama untuk diputuskan
  dan diuji).
- **File yang perlu disentuh**: `internal/modules/organization/repository/*.go`,
  `internal/modules/subscription/repository/*.go`, `internal/modules/billing/repository/*.go`.
- **Risiko**: rendah (read-only audit), tapi temuan bisa memicu perubahan kode.
- **Acceptance criteria**: checklist review selesai, setiap query List/Get yang menerima `organizationID`
  terverifikasi memakainya di klausa WHERE.
- **Test**: tidak perlu test baru untuk audit itu sendiri; temuan bug (jika ada) butuh regression test.

## P1 — Penting (Kualitas & Konsistensi)

### P1-1. Lengkapi dokumentasi OpenAPI untuk endpoint Permission/Role Management
- **Alasan**: `admin/permissions*`, `admin/roles*`, `admin/permission-matrix`, `admin/users/:id/permissions*`,
  `admin/users/:id/roles*` sudah ada di kode (`internal/core/permission/handler/`) tapi tidak terdokumentasi
  di `api/openapi.yaml` (lihat [api-contract-review.md](api-contract-review.md)).
- **File**: `api/openapi.yaml`.
- **Risiko**: rendah (dokumentasi saja), tapi konsumen API eksternal/frontend tidak tahu endpoint ini ada.
- **Acceptance criteria**: semua path di atas terdokumentasi lengkap (request/response/error) di OpenAPI,
  tervalidasi dengan linter OpenAPI (jika ada) atau minimal `openapi.yaml` tetap valid YAML/schema.
- **Test**: tidak perlu test kode; opsional tambahkan contract test jika repo punya tooling untuk itu.

### P1-2. Audit request/response body per endpoint vs DTO Go
- **Alasan**: perbandingan yang sudah dilakukan baru level path, bukan body — ada risiko drift field
  request/response yang tidak terdeteksi (lihat [api-contract-review.md](api-contract-review.md) bagian 3).
- **File**: `api/openapi.yaml` vs `internal/modules/*/dto/{request,response}.go`.
- **Risiko**: rendah-menengah — drift dokumentasi bisa menyesatkan integrasi frontend/eksternal.
- **Acceptance criteria**: minimal modul `landing` dan `billing` (paling aktif berubah) sudah diaudit dan
  mismatch dikoreksi.
- **Test**: tidak perlu test kode.

### P1-3. Lengkapi integrasi payment gateway Xendit di modul billing
- **Alasan**: modul billing WIP, `internal/platform/xendit/` sudah ada sebagai client tapi alur
  invoice→payment→webhook belum lengkap sesuai temuan module-map.md.
- **File**: `internal/modules/billing/service/payment_service.go`, `internal/modules/billing/handler/`,
  `internal/platform/xendit/`.
- **Risiko**: menengah — menyentuh alur uang, butuh idempotency ketat (`internal/core/idempotency`) dan
  test webhook yang mensimulasikan retry/duplicate event (constraint unique `billing_payment_events` sudah
  ada sebagai pengaman DB level).
- **Acceptance criteria**: sesuai `docs/billing-plan-development-tasks.md` (sudah ada breakdown detail di
  sana — task ini hanya menegaskan prioritas dari sisi repository-wide review).
- **Test**: unit test untuk idempotency webhook, integration test untuk flow invoice→paid.

### P1-4. Verifikasi runtime enforcement scope granular RBAC (`own`/`team`/`branch`/`department`)
- **Status saat ini (dikonfirmasi manual 2026-07-01)**: "RBAC scope exists in database schema, including
  organization and all, but runtime enforcement needs code verification." Scope `organization`/`all` sudah
  **Confirmed** ada di schema dan selaras dengan middleware yang ada; scope granular lain (`own`/`team`/
  `branch`/`department`) berstatus **Needs code verification**.
- **Alasan**: kolom `role_permissions.scope` punya nilai ini tapi belum terkonfirmasi dicek di
  `internal/core/permission/policy/checker.go` (lihat [permission-context.md](permission-context.md)).
- **File**: `internal/core/permission/policy/checker.go`, `internal/core/permission/service/`.
- **Risiko**: rendah — task verifikasi, bisa memicu keputusan desain (implementasikan atau hapus opsi
  scope yang tidak dipakai dari constraint CHECK).
- **Acceptance criteria**: dokumentasi [permission-context.md](permission-context.md) diupdate dengan
  kesimpulan pasti — status berubah dari "Needs code verification" menjadi "Confirmed" (jika ternyata sudah
  diimplementasikan) atau "Confirmed: not implemented, reserved for future use" (jika belum, dan itu memang
  keputusan sadar).
- **Test**: jika ternyata belum diimplementasikan dan diputuskan perlu, tambahkan unit test checker untuk
  tiap nilai scope.

### P1-5. Buat config minimal `.golangci.yml`
- **Status saat ini (dikonfirmasi manual 2026-07-01)**: "golangci-lint is intended to be used, but in-repo
  configuration needs verification or creation." Dicek ulang: tidak ada `.golangci.yml`/`.yaml`/`.toml`/
  `.json` di root repo — **Confirmed absent**, config belum pernah dibuat.
- **Alasan prioritas**: tanpa config, tidak ada standar quality gate yang konsisten antar kontributor/CI di
  masa depan.
- **File yang perlu disentuh**: buat `.golangci.yml` baru di root (lihat contoh minimal di
  [development-guide.md](development-guide.md) bagian 8).
- **Risiko**: rendah — file config baru, tidak mengubah kode. Risiko sedang jika linter set yang dipilih
  langsung mengaktifkan aturan strict yang memunculkan banyak temuan sekaligus di kode existing — mulai dari
  linter set minimal (`govet`, `staticcheck`, `unused`, `errcheck`) lalu perluas bertahap.
- **Acceptance criteria**: `.golangci.yml` ada di repo, `golangci-lint run` bisa dieksekusi tanpa error
  konfigurasi (temuan lint boleh ada, itu task lanjutan terpisah untuk membersihkannya).
- **Test**: tidak berlaku (config, bukan kode).

### P1-6. Bersihkan/arsipkan `api/openapi-landing.yaml`, jadikan `api/openapi.yaml` source of truth tunggal
- **Status saat ini (dikonfirmasi manual 2026-07-01)**: "api/openapi.yaml is the canonical OpenAPI source.
  Landing OpenAPI definitions should be merged into api/openapi.yaml." Isi `api/openapi-landing.yaml` (stub
  draft 22 baris, 2 path) sudah **Confirmed** sepenuhnya tercakup di `api/openapi.yaml` — tidak ada konten
  baru yang perlu dipindahkan.
- **File yang perlu disentuh**: hapus `api/openapi-landing.yaml` (atau pindahkan ke `docs/archive/` jika
  ingin disimpan sebagai riwayat), pastikan tidak ada referensi lain ke file ini di tooling/CI/skrip mana pun
  sebelum dihapus (`grep -r "openapi-landing" .`).
- **Risiko**: sangat rendah — file stub tidak dipakai runtime (bukan kode Go), aman dihapus setelah
  konfirmasi tidak direferensikan tooling lain.
- **Acceptance criteria**: hanya `api/openapi.yaml` yang tersisa sebagai spec resmi; jika tim tetap butuh
  pemisahan modular OpenAPI di masa depan, dokumentasikan sebagai keputusan baru terpisah (bukan
  membangkitkan kembali `openapi-landing.yaml` yang lama).
- **Test**: tidak berlaku (dokumentasi/config).

## P2 — Peningkatan (Tidak Mendesak, Tapi Bernilai)

### P2-1. Bersihkan atau dokumentasikan folder `internal/modules/landingpage/`
- **Alasan**: folder kosong (0 file) berpotensi membingungkan developer baru yang mencari modul landing page
  (modul aktif adalah `internal/modules/landing/`).
- **File**: `internal/modules/landingpage/` (hapus) atau tambahkan `doc.go` yang menjelaskan status legacy.
- **Risiko**: sangat rendah — folder kosong tidak dikompilasi/diimpor di manapun (perlu konfirmasi cepat
  dengan `grep -r "modules/landingpage" internal/` sebelum menghapus).
- **Acceptance criteria**: folder dihapus ATAU diberi `doc.go` penjelasan, dan `go build ./...` tetap sukses.
- **Test**: `go build ./...` setelah perubahan.

### P2-2. Standardisasi helper pagination bersama
- **Alasan**: `internal/shared/pagination` baru berisi doc comment, setiap modul implementasi ad-hoc
  (`Limit`/`Offset` di masing-masing filter struct) — lihat [api-contract-review.md](api-contract-review.md)
  bagian 5.
- **File**: `internal/shared/pagination/*.go` (implementasi baru), lalu migrasi bertahap modul existing.
- **Risiko**: menengah jika dipaksakan sekaligus (bisa mengubah kontrak response banyak endpoint) — harus
  bertahap per modul, dan idealnya tanpa breaking change response (`meta` field tetap kompatibel).
- **Acceptance criteria**: helper pagination baru dipakai minimal di 1 modul baru sebagai proof of concept,
  tanpa mengubah modul existing dulu (langkah awal, non-breaking).
- **Test**: unit test helper pagination baru.

### P2-3. Klarifikasi tumpang tindih modul stub vs modul aktif
- **Status**: sebagian sudah selesai — stub `product` (dulu tumpang tindih dengan `billing_plans`/
  `billing_features`) sudah **resolved** lewat refactor domain-split 2026-07-01: sekarang jadi modul aktif
  `internal/modules/product/` sendiri, bukan lagi stub yang tumpang tindih.
- **Alasan (sisa)**: `payment` (stub) vs `billing` (sudah punya payment nyata); `order` (stub) vs
  `customer_subscriptions`/`billing_invoices` — lihat [module-map.md](module-map.md).
- **File**: tidak ada perubahan kode — ini task keputusan arsitektur/scope, didokumentasikan di
  `docs/reference-<modul>.md` baru untuk masing-masing modul stub yang akan digarap.
- **Risiko**: rendah sekarang (belum ada kode), tinggi jika diabaikan sampai modul stub mulai diimplementasi
  tanpa scope jelas (bisa berujung duplikasi logic).
- **Acceptance criteria**: sebelum modul stub manapun mulai digarap, ada dokumen scope yang eksplisit
  membedakan tanggung jawabnya dari modul aktif terkait.
- **Test**: tidak berlaku (dokumentasi).

### P2-4. Tambahkan anotasi permission requirement ke OpenAPI
- **Alasan**: saat ini permission slug per endpoint hanya bisa dilihat dari kode, tidak dari API spec —
  lihat [api-contract-review.md](api-contract-review.md) bagian 6.
- **File**: `api/openapi.yaml` (tambahkan extension field atau deskripsi).
- **Risiko**: sangat rendah.
- **Acceptance criteria**: minimal endpoint admin (landing/organization/billing/user) punya anotasi
  permission di spec.
- **Test**: tidak berlaku.

## Catatan Prioritas P0 sudah selesai sebagian sebagai bagian paket dokumentasi ini

Koreksi `README.md` dan `AGENTS.md` (menghapus klaim fitur/teknologi yang tidak ada) sudah dikerjakan
bersamaan dengan pembuatan 8 dokumen ini — lihat riwayat git kedua file tersebut. Task P0/P1/P2 di atas
adalah task **kode dan verifikasi lanjutan**, bukan dokumentasi.

## Follow-up Development Tasks (dari Verifikasi Manual 2026-07-01)

Ringkasan task konkret yang muncul dari 5 keputusan manual pemilik repo (detail lengkap masing-masing ada
di badan dokumen ini dan di [repository-context.md](repository-context.md) bagian 10):

| Task | Prioritas | Alasan Singkat | Referensi |
|---|---|---|---|
| Buat config minimal `.golangci.yml` | P1 | Confirmed: config belum ada di repo, golangci-lint akan dipakai sebagai quality gate | P1-5 di atas, [development-guide.md](development-guide.md) §8 |
| Audit runtime enforcement scope granular RBAC (`own`/`team`/`branch`/`department`) di `internal/core/permission/policy/checker.go` | P1 | Needs code verification — scope ada di schema, enforcement belum dibuktikan | P1-4 di atas, [permission-context.md](permission-context.md) |
| Hapus/arsipkan `api/openapi-landing.yaml`, jadikan `api/openapi.yaml` satu-satunya source of truth | P1 | Confirmed: isi stub sudah tercakup penuh di openapi.yaml, tidak ada yang perlu di-merge ulang | P1-6 di atas, [api-contract-review.md](api-contract-review.md) |
| Tidak ada aksi kode untuk Redis cache/queue | — | Deferred — Redis session-only adalah keputusan final untuk fase ini, bukan gap yang perlu ditutup sekarang | [permission-context.md](permission-context.md), [repository-context.md](repository-context.md) §10 |
| Tidak ada aksi kode untuk database-per-tenant (`data_placement=dedicated`) | — | Deferred (future capability) — fokus tetap shared database + isolasi `organization_id`/`tenant_id` | [database-context.md](database-context.md) §8, P0-1/P0-2 di atas |

Dua baris terakhir sengaja **tanpa task kode** — keduanya adalah keputusan scope resmi (Redis session-only,
shared database dulu) yang justru **menegaskan** bahwa audit filter tenant manual (P0-1/P0-2) dan RLS untuk
`organization_*`/`billing_*` tetap jadi prioritas utama, karena tidak ada rencana mitigasi tambahan lewat
Redis cache atau isolasi database terpisah dalam waktu dekat.

## Follow-up: Frontend Integration (dari Refactor Domain-Split Product/Subscription/Billing, 2026-07-01)

Refactor domain-split modul `billing` → `product`/`subscription`/`billing` (lihat
[product-subscription-billing-concept.md](product-subscription-billing-concept.md)) mengubah route API
platform-side (`/api/v1/platform/billing/plans*`/`features*` → `/api/v1/platform/product/*`;
`/api/v1/platform/billing/subscriptions*` → `/api/v1/platform/subscriptions*`) dan permission slug terkait.
Keputusan eksplisit: **frontend TIDAK diupdate dalam paket refactor ini** — repo `frontend.zyad.cloud` sudah
punya integrasi nyata ke route lama dan akan patah begitu backend di-deploy.

| Task | Prioritas | Alasan | File (repo `frontend.zyad.cloud`) |
|---|---|---|---|
| Update path endpoint plan/feature/entitlement | **P1 (blocking sebelum deploy backend)** | Route berubah total, request ke path lama akan 404 | `src/features/billing/api/platform-billing.api.ts` |
| Update path endpoint subscription | **P1 (blocking sebelum deploy backend)** | Route berubah total | `src/features/billing/api/platform-billing.api.ts` |
| Update halaman yang memanggil API di atas | **P1** | Konsumen langsung dari file API di atas | `src/features/billing/pages/PlatformBillingPlansPage.vue`, query hooks di `platform-billing.queries.ts` |
| Verifikasi `billing.api.ts` & `PlanUpgradePage.vue` | P2 (verifikasi saja) | Route `/app/billing/*` TIDAK berubah, seharusnya tidak perlu diedit — tapi verifikasi tetap dianjurkan | `src/features/billing/api/billing.api.ts`, `src/features/billing/pages/PlanUpgradePage.vue` |

**Acceptance criteria**: sebelum backend hasil refactor ini di-deploy ke environment yang diakses frontend,
task di atas harus selesai — jika tidak, halaman admin plan/feature/subscription di frontend akan error.
