# Paket Free: definisi dan migrasi tenant lama

Definisi nilai Free ada di `internal/app/free_plan.go` (`FreePlanFeatures()`), mengikuti spec
`docs/superpowers/specs/2026-10-22-entitlement-upgrade-ux-design.md` §4. Produk katalog platform ber-SKU
`SELF_SERVE_FREE_PRODUCT_SKU` (default `FREE`) **harus sudah ada** di environment target; `free-plan` gagal
dengan error bila produk itu tidak ditemukan (buat dulu lewat UI katalog).

## Perilaku key kuota baru

Delapan key kuota baru (`crm.max_leads`, `crm.max_companies`, `crm.max_deals`, `crm.max_pipelines`,
`crm.max_quotations_per_month`, `crm.max_sales_orders_per_month`, `receivable.max_invoices_per_month`,
`domain.max_domains`; daftar tunggal: `organizationservice.UncappedWhenAbsentQuotaKeys()`):

- organisasi **tanpa** baris entitlement untuk key itu: **tidak dibatasi** (create diizinkan, summary `unlimited`);
- baris dengan `enabled=false`: `FEATURE_NOT_ENABLED`;
- baris dengan limit: ditegakkan (`QUOTA_EXCEEDED` saat penuh).

Key lama (`crm.max_contacts`, `landing.max_pages`, `users.max_users`, `storage.max_bytes`, …) tidak berubah.
Akibatnya men-deploy kode ini **sebelum** `free-plan --apply` tidak mengunci siapa pun: tenant Free hanya
belum dibatasi sampai `--apply` dijalankan.

## Prasyarat (wajib sebelum deploy ke environment mana pun)

1. Test integrasi meter dan migration up/down **belum pernah dijalankan di mana pun**. Jalankan di database
   scratch (env `TEST_DB_*`, lihat README "Integration Test Database"), bukan DB dev/produksi:

   ```bash
   cd backend
   go test -tags integration ./internal/app/ -run Meter
   ```

   lalu uji `migrate` up → down → up untuk `000158`–`000160` di database scratch yang sama.
2. Produk `FREE` ada di katalog platform environment target.
3. Pastikan key berikut **ACTIVE** di registry environment target: `crm.enabled`, `receivable.enabled`,
   `landing.enabled`, `domain.enabled`, `whatsapp.enabled`. Frontend menyembunyikan menu modul yang key-nya
   tidak muncul di summary (summary hanya memuat fitur registry yang aktif).

## Urutan rollout

1. **Deploy release.** Migration `000158`–`000160` hanya menambah registry fitur dan ikut otomatis:
   `deploy/deploy.sh` menjalankan `./migrate -direction up` sebelum `pm2 startOrReload`. Bukan langkah manual.
2. `accessmigrate audit-quota` (baca-saja, informasional): daftar produk non-FREE dan organisasi
   contract/plan yang belum membawa key kuota baru — mereka **uncapped** untuk key tersebut, tidak terkunci.
   Tambahkan key ke produk/contract hanya bila tenant itu memang ingin dibatasi (keputusan bisnis).
3. `accessmigrate free-plan` (DRY-RUN, default): tinjau perubahan fitur produk FREE dan limit per organisasi.
   Baca baris `-> (none)`: key itu akan **dihapus** dari produk FREE.
4. `accessmigrate free-plan --apply`:
   - mengganti **seluruh** set fitur produk FREE dengan `FreePlanFeatures()`;
   - memanggil `ProvisionDefaultAccess` untuk organisasi ber-akses `default` yang berubah; entitlement
     sumber `default` untuk key yang tidak ada di `FreePlanFeatures()` di-expire;
   - organisasi dengan contract aktif, organisasi plan, dan organisasi platform tidak disentuh;
   - tenant yang sudah melebihi limit baru tetap menyimpan semua datanya; mereka hanya tidak bisa membuat data
     baru sampai di bawah limit atau upgrade;
   - mencetak `APPLIED` hanya bila semua organisasi berhasil; kegagalan sebagian mencetak `done/total` dan
     keluar dengan error (aman diulang, idempoten).

Jalankan dari `backend/` dengan `.env` environment target, mis. `go run ./cmd/accessmigrate free-plan`.

## Rollback

- Kode: kembalikan ke release sebelumnya (symlink release lama + `pm2 startOrReload`).
- Set fitur FREE: mengedit produk di UI katalog **tidak** me-re-provision organisasi yang sudah ada (entitlement
  mereka snapshot yang dibekukan); satu-satunya re-provision massal adalah `free-plan --apply`, dan nilainya
  diambil dari **kode** (`FreePlanFeatures()`), bukan dari katalog. Jadi memulihkan set lama berarti
  menerapkan **nilai sebelumnya** di kode (simpan output dry-run sebelum `--apply`) lalu menjalankan
  `--apply` dengan kode itu. Jangan "pulihkan lewat UI lalu `--apply`" — `--apply` akan menimpanya lagi.
- **Jangan pernah** menjalankan migration down `000158`–`000160` selama kode ini live: untuk organisasi yang
  **punya** baris entitlement untuk key itu, key yang hilang dari registry membuat setiap create yang dijaga
  kuota gagal 500 `QUOTA_ENFORCER_MISCONFIGURED`. Organisasi tanpa baris untuk key baru tidak terdampak (sudah
  diputuskan uncapped sebelum lookup registry).

## Celah yang diketahui / keputusan sadar

- Pembuatan subdomain tidak digerbang `domain.enabled` (hanya custom domain). Organisasi **tanpa** baris efektif
  `domain.max_domains` (mis. provisioning default gagal — tercatat di log, tidak di-retry) dapat membuat subdomain
  tanpa batas — sama seperti sebelum fitur ini.
- Restore contact kini dicek kuota `crm.max_contacts` (seperti lead/company/deal/pipeline). Restore halaman
  landing tidak perlu dicek: itu hanya mengubah status `archived` → `draft` pada halaman yang belum dihapus,
  dan halaman arsip sudah dihitung oleh `landing.max_pages`; halaman yang dihapus (soft delete) tidak punya
  jalur restore.

## Catatan receivable.online_payment

`receivable.online_payment` ditambahkan ke paket Free (keputusan pemilik produk, 23 Okt 2026; spec
`2026-10-23-tenant-products-entitlement-design.md` §8 keputusan 19): tenant Free dapat menagih pelanggannya secara
online lewat checkout mandiri, dan semua pembayaran tetap masuk ke DOKU Zyad. Key sudah ada di registry, tanpa migration.

- Perubahan ini baru berlaku di produksi setelah `accessmigrate free-plan` dry-run ditinjau, lalu `--apply`
  dijalankan **atas permintaan eksplisit pemilik**. Agen/CI tidak boleh menjalankannya sendiri.
- Tenant Free yang sudah ada **tidak berubah** sampai `--apply`; dry-run akan menampilkan baris
  `receivable.online_payment (none) -> true`.

## Catatan WhatsApp

`whatsapp.enabled` dipertahankan di paket Free (keputusan pemilik produk, 23 Okt 2026): produk FREE produksi
sudah memakainya dan modulnya ditegakkan kode (gerbang rute `/app/whatsapp` dan pengiriman dokumen).
`FreePlanFeatures()` tidak membawa `whatsapp.max_sessions`, jadi sesi WhatsApp tidak dibatasi paket Free. Dry-run
produksi sebelum perubahan ini menunjukkan `whatsapp.enabled true -> (none)`; setelah perubahan, baris itu tidak
muncul lagi.

## Catatan storage

Modul asset menegakkan `storage.max_bytes` (satuan byte); `media.max_storage_mb` tidak ditegakkan di kode mana pun.
Free mengisi keduanya (500 MB) agar konsisten. Baris `storage.max_bytes` yang diset manual (mis. `platform_override`)
tidak ditimpa oleh provisioning `default`.
