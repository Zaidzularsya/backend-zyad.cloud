# Paket Free: definisi dan migrasi tenant lama

Definisi nilai Free ada di `internal/app/free_plan.go` (`FreePlanFeatures()`), mengikuti spec
`docs/superpowers/specs/2026-10-22-entitlement-upgrade-ux-design.md` §4. Produk katalog platform ber-SKU
`SELF_SERVE_FREE_PRODUCT_SKU` (default `FREE`) harus sudah ada; subcommand hanya menyelaraskan fiturnya.

## Urutan rollout

1. Jalankan migration sampai `000160` (registry key kuota baru + `storage.max_bytes`).
2. `accessmigrate audit-quota` (baca-saja): daftar produk/contract berbayar yang belum membawa key kuota baru.
   Tenant tersebut menerima `FEATURE_NOT_ENABLED` saat membuat data terkait sampai kuota diberikan.
3. `accessmigrate free-plan` (DRY-RUN secara default): tinjau perubahan fitur produk FREE dan limit per organisasi.
4. `accessmigrate free-plan --apply`: memperbarui fitur produk FREE lalu memanggil `ProvisionDefaultAccess`
   untuk organisasi ber-akses `default` yang berubah. Idempoten; workspace dengan contract aktif dilewati.

Jalankan dari `backend/` dengan `.env` environment target (`go run ./cmd/accessmigrate free-plan`).
Rollback: kembalikan fitur produk FREE lewat UI katalog lalu jalankan `free-plan --apply` lagi.

## Catatan storage

Modul asset menegakkan `storage.max_bytes` (satuan byte); `media.max_storage_mb` tidak ditegakkan di kode mana pun.
Free mengisi keduanya (500 MB) agar konsisten. Baris `storage.max_bytes` yang diset manual (mis. `platform_override`)
tidak ditimpa oleh provisioning `default`.
