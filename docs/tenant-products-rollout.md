# Produk tenant: rollout, konfigurasi, dan runbook

Spec: `docs/superpowers/specs/2026-10-23-tenant-products-entitlement-design.md`. Dokumen ini melengkapi
`free-plan-rollout.md` (paket Free) dan hanya membahas apa yang berubah karena fitur "produk tenant".

## Masalah dan hasil akhir

Sebelum ini hanya katalog platform yang bisa memberi entitlement. Setelah fitur ini:

- Tenant yang punya izin `catalog_product.manage_entitlements` dapat melampirkan fitur yang di-flag
  `tenant_offerable` (dengan batas atas `tenant_max` untuk fitur integer) pada produk miliknya.
- Pelanggan tenant dapat membeli produk lewat halaman publik `/checkout/:productId` di host tenant
  (Google sign-in, invoice online lewat DOKU Zyad), tanpa login dan tanpa campur tangan staf.
- Reseller dengan fitur `landing.page_activation` dapat menerbitkan invoice aktivasi untuk satu halaman
  GrapesJS; halaman terbit otomatis saat invoice lunas dan berakhir pada `active_until`.
- Semua pembayaran online masuk ke merchant DOKU Zyad. Ledger `settlement_entries` mencatat hak tiap
  tenant/reseller atas uang itu; super admin mencatat setoran manual di halaman "Saldo & Setoran".

## Status verifikasi (baca dulu)

- Yang sudah dijalankan agen: unit test Go, `go vet`, `go build`, unit test + typecheck + build frontend, dan
  e2e Playwright dengan API di-stub (`frontend/tests/e2e/tenant-products.spec.ts`).
- **Belum pernah dijalankan oleh siapa pun**: migration `000161`–`000166` (up maupun down), semua test
  `-tags integration` (Task 7, 10, 13, 14, 15, 16: lock advisory, FK komposit, `ListBalances`, settle), dan
  alur nyata dengan DOKU sandbox/Google. Semua klaim perilaku SQL di bawah berasal dari membaca kode.
- Task 11b (undangan staf lewat email) **tidak dibangun** (ditunda); lihat "Keterbatasan".

## Prasyarat

1. Jalankan test integrasi di database scratch (env `TEST_DB_*`, lihat README "Integration Test Database"),
   bukan DB dev/produksi:

   ```bash
   cd backend
   go test -tags integration ./internal/app/ ./internal/modules/...
   ```

   lalu uji `migrate` up → down → up untuk `000161`–`000166` di database scratch yang sama.
2. Produk `FREE` ada di katalog platform (sudah syarat `free-plan-rollout.md`).
3. Organisasi platform dapat di-resolve (checkout tenant memakai scope platform untuk produk Free/tautan
   workspace).

## Migration (urutan, semuanya aditif)

Nama file nyata di `backend/migrations`:

| No | File | Isi |
| --- | --- | --- |
| 000161 | `tenant_offerable_features` | Kolom `product_features.tenant_offerable` (default `false`) dan `tenant_max` (nullable, CHECK > 0, integer offerable wajib punya `tenant_max`); seed fitur boolean `landing.page_activation` (`ON CONFLICT DO NOTHING`). Default membuat semua fitur lama **tidak** offerable: tidak ada perubahan perilaku. |
| 000162 | `catalog_product_manage_entitlements_permission` | Izin `catalog_product.manage_entitlements` untuk role `organization_owner` dan `super_admin`. |
| 000163 | `landing_page_activation` | Kolom `landing_pages.active_until` (NULL = tidak pernah diaktivasi, perilaku lama) dan tabel `landing_page_activations` (status `open`/`paid`/`void`, hash token preview, FK komposit ke halaman dan invoice milik org yang sama). |
| 000164 | `landing_activation_paid_template` | Template notifikasi email `landing.activation_paid` (sistem, upsert). |
| 000165 | `settlement_ledger` | Tabel `settlement_entries` (`payment`/`settlement`/`adjustment`), unik per `provider_reference` untuk `payment`. Sengaja **tanpa RLS** (pembukuan platform lintas tenant), akses dibatasi di service. FK organisasi tanpa `ON DELETE CASCADE`. |
| 000166 | `settlement_permissions` | `settlement.manage` (super admin) dan `settlement.read` (owner org, saldo miliknya sendiri). |

Karena aditif/registry-only, migration boleh dirilis **sebelum** kode. `deploy/deploy.sh` menjalankan
`./migrate -direction up` sebelum `pm2 startOrReload`, jadi pada deploy normal ini otomatis.

## Urutan rilis

1. Backup database (penting: ini pertama kali `settlement_entries` ada; tidak ada data lama, tetapi
   backup tetap prosedur standar).
2. Terapkan migration `000161`–`000166` (sendiri atau lewat `deploy.sh`).
3. Deploy kode backend, lalu frontend (symlink release + `pm2 startOrReload`; lihat `vps-deploy` /
   skill `zyad-dev-deploy` untuk dev).
4. **Set flag katalog sebelum tenant dapat menjual fitur apa pun.** Di Fitur Platform (super admin), untuk
   tiap fitur yang boleh dijual tenant: aktifkan `tenant_offerable`, dan untuk fitur integer isi `tenant_max`
   (wajib; CHECK database menolak offerable integer tanpa max). Selama belum ada fitur offerable, tenant
   hanya dapat membuat produk tanpa entitlement. Tentukan juga siapa yang diberi `landing.page_activation`
   (lewat produk/contract), karena tombol "Create invoice" hanya muncul untuk yang punya fitur itu.
5. `accessmigrate free-plan` **DRY-RUN** (default): tinjau barisnya. Harus muncul
   `receivable.online_payment (none) -> true` (ditambahkan ke Free oleh perubahan ini; lihat
   `free-plan-rollout.md` "Catatan receivable.online_payment").
6. `accessmigrate free-plan --apply` **hanya atas permintaan eksplisit pemilik**. Agen/CI tidak boleh
   menjalankannya. Sebelum `--apply` tenant Free yang ada tidak berubah.

## Checklist konfigurasi (pemilik)

- **Google client id**: `AUTH_GOOGLE_CLIENT_IDS` (backend) dan `VITE_GOOGLE_CLIENT_ID` (frontend) harus
  menyertakan client yang OAuth-nya mengizinkan *Authorized JavaScript origins* untuk **setiap origin domain
  kustom tenant** yang akan memakai checkout (mode popup GIS dipicu dari origin halaman). Subdomain wildcard
  tidak didukung Google: tiap domain kustom harus didaftarkan manual. Belum diverifikasi di browser nyata.
- **`TRUSTED_PROXY_CIDRS`** (`TrustedProxyCIDRs`): isi dengan CIDR proxy (nginx/Cloudflare) agar rate limit
  per-IP checkout (10 per 10 menit) memakai IP klien sungguhan. Tanpa ini semua pembeli bisa berbagi satu
  kunci IP (atau IP proxy) dan terblokir bersama.
- **`APP_FRONTEND_URL`**: link invoice publik (`/i/:token`) dibangun dari nilai ini, bukan dari host tenant.
  Frontend checkout memang menerima redirect lintas-origin ke host ini; pastikan nilainya benar.
- **nginx**: bila `preview_url` dipakai, host tenant harus meneruskan `/api/v1/public/*` ke backend.
  `preview_url` yang dikembalikan saat ini adalah **path API** (`https://<host>/api/v1/public/landing/preview/<token>`),
  belum ada rute SPA untuk pembeli.
- **Cloudflare/cache**: `GET /public/tenant-catalog` mengirim `Cache-Control: public, max-age=60` dengan
  `Vary: Host, X-Forwarded-Host`. Pastikan cache edge menghormati `Vary: Host`; kalau tidak, katalog satu
  tenant bisa disajikan ke tenant lain. Jika ragu, jangan cache path ini di edge.

## Rollback

- **Kode**: kembalikan symlink ke release sebelumnya + `pm2 startOrReload`. Migration aditif tidak perlu
  dibalik.
- **Jangan menjalankan migration down selama kode baru live.** Khususnya `000161` (hapus kolom/fitur yang
  dipakai query katalog), `000163`/`000165` (hapus tabel berisi aktivasi dan catatan uang). Down `000165`
  menghapus pembukuan uang yang tidak bisa direkonstruksi dari tempat lain selain laporan DOKU.
- Down `000161` menghapus baris `landing.page_activation`; periksa dulu bahwa tidak ada entitlement yang
  merujuknya.
- `free-plan --apply` tidak punya rollback otomatis; pulihkan nilai sebelumnya lewat kode seperti dijelaskan
  di `free-plan-rollout.md`.

## Runbook operasional untuk celah yang diketahui

### i. Settle gagal setelah pembayaran ter-commit (aktivasi tidak terbit)

Pembayaran dicatat dulu, baru listener aktivasi menjalankan settle. Bila settle error, listener hanya
mencatat log dan **tidak ada retry otomatis**: invoice `paid`, aktivasi tetap `open`, halaman belum terbit.

- Cari di log API: `activation payment: settle failed` (field `invoice_id`, `error`). Varian lain yang perlu
  dicek: `activation payment received but page was not published` (WARN; halaman tidak ditemukan/diarsipkan).
  Jangan cari di log worker: listener hanya terdaftar di proses API.
- Pemulihan: kirim ulang notifikasi DOKU untuk invoice itu (dari dashboard DOKU / minta retry) dengan
  **referensi pembayaran yang sama**. Pembayaran yang sudah tercatat tidak dicatat ulang (`created=false`);
  karena invoice sudah `paid`, receivable memanggil ulang **hanya** listener yang opt-in
  `ReplayableListener` (saat ini hanya listener aktivasi landing), yang menjalankan `SettleByInvoice` lagi:
  aktivasi `open`/`void` menjadi `paid`, halaman terbit, snapshot diambil, notifikasi pembeli dikirim sekali.
  Bila aktivasi sudah `paid`, panggilan ulang tidak mengubah apa pun dan tidak mengirim notifikasi kedua.
  Listener CRM (Won/contract) dan render ulang snapshot invoice **tidak** dijalankan ulang.
  Catatan: `paid_at` aktivasi pada pemulihan ini = waktu notifikasi ulang diproses, bukan waktu bayar asli
  (periode aktif pembeli bergeser maju sebesar jeda itu). Diuji dengan unit test; belum diuji dengan DOKU
  sandbox nyata.
- Alternatif manual (hanya panduan, **jangan dijalankan tanpa persetujuan pemilik**): tentukan dulu keadaan
  dengan SELECT.

  ```sql
  SELECT a.id, a.page_id, a.status, a.invoice_id, a.paid_at, i.status AS invoice_status
  FROM landing_page_activations a
  JOIN invoices i ON i.id = a.invoice_id
  WHERE a.status = 'open' AND i.status = 'paid';
  ```

  Memperbarui `landing_page_activations.status/paid_at`, `landing_pages.status/published_at/active_until`
  dan snapshot versi secara manual rawan salah (aturan `active_until = GREATEST(active_until saat ini jika
  masih di masa depan, paid_at + durasi)` dan snapshot ada di kode). Lebih aman menunggu retry notifikasi
  atau meminta perbaikan lewat kode.

### ii. Uang masuk tetapi tidak tercatat permanen di ledger

Hanya ada log ERROR (tanpa tabel penampung) pada kasus berikut. Rekonsiliasi terhadap laporan settlement
DOKU:

- `receivable: pembayaran DOKU untuk invoice non-payable, perlu refund manual`: invoice sudah lunas/batal
  atau pembayaran melebihi sisa. Pembayaran **tidak** dicatat di receivable maupun ledger. Termasuk
  pembayaran ganda pada invoice yang sudah lunas.
- `receivable: settlement ledger skipped for non-IDR invoice`: ledger tidak punya kolom mata uang; invoice
  non-IDR sengaja tidak dikreditkan.
- `receivable: settlement ledger entry failed` dan `settlement provider reference collision`: payment sah
  tetapi entri ledger gagal/bentrok referensi. Pemulihan: kirim ulang notifikasi (entri ledger unik per
  referensi, jadi aman diulang); bila bentrok antar-organisasi, selidiki manual.

Prosedur: ambil laporan DOKU periode terkait, cocokkan per nomor invoice/referensi dengan
`settlement_entries` (`kind='payment'`) dan log di atas; selisihnya diputuskan pemilik (refund atau
penyesuaian manual `adjustment` bernote).

### iii. Prosedur setoran manual (super admin)

1. Buka `/platform/settlements` ("Saldo & Setoran"; izin `settlement.manage`). Cari organisasi, klik
   "Kelola".
2. Pastikan DOKU benar-benar sudah mencairkan/ Anda sudah mentransfer ke tenant, lalu pilih "Setoran",
   isi jumlah, tanggal (WIB) dan catatan, konfirmasi. Catatan keuangan tidak dapat diubah/dihapus.
3. Aturan (ditegakkan server di bawah lock per organisasi): setoran > 0 dan **tidak boleh melebihi sisa
   saldo** (`422 SETTLEMENT_EXCEEDS_BALANCE`). "Penyesuaian" boleh negatif, wajib catatan, dan tidak boleh
   membuat sisa saldo di bawah nol (`422 ADJUSTMENT_BELOW_ZERO`). Hanya organisasi bertipe customer yang
   dapat menerima.
4. Owner tenant melihat saldo dan riwayatnya sendiri di `/app/billing/settlement` (izin `settlement.read`).

Koreksi kesalahan input dilakukan dengan entri `adjustment` lawan, bukan edit/hapus.

### iv. Pembayaran ganda

- Dua pembayaran pada **invoice yang sama**: pembayaran kedua ditolak receivable sebagai non-payable (lihat
  ii); aktivasi/ledger tidak bertambah, uangnya ada di DOKU. Keputusan refund ada pada pemilik.
- Pembayaran invoice lama yang sudah lewat jatuh tempo setelah invoice pengganti dibuat: aktivasi `void`
  tetap diterima dan halaman terbit; `active_until` memakai `GREATEST(...)` sehingga periode **tidak pernah
  memendek**, tetapi dua invoice berbayar menghasilkan satu perpanjangan yang dihitung dari masing-masing
  `paid_at`, bukan dua periode penuh berurutan. Kebijakan refund/kompensasi tetap keputusan pemilik.

### v. Keterbatasan yang diketahui

- Checkout tenant hanya untuk produk **berulang + prabayar**; tidak ada pascabayar (spec §4.13 belum
  dipenuhi).
- Tidak ada undangan staf lewat email (Task 11b ditunda). Staf tetap dibuat lewat CRM lalu ditautkan.
- Tidak ada override katalog per-tenant; flag `tenant_offerable/tenant_max` berlaku global.
- `preview_url` berupa path API; belum ada rute SPA untuk pembeli. Link preview hanya muncul sekali.
- Endpoint publik (`/public/tenant-catalog`, preview, dll.) tidak punya rate limit khusus; hanya
  `POST /public/tenant-checkout` yang dibatasi (per IP dan per tenant, Redis). Bila Redis tidak tersedia,
  limiter **fail-open**.
- Tenant yang tidak punya pipeline tidak dapat menerima self-checkout (`ErrSelfServeNotConfigured`).
- Kegagalan snapshot/notifikasi setelah aktivasi hanya WARN; halaman tetap terbit.
- Defek lama terpisah (bukan bagian rilis ini): respons publik landing dapat memuat `PasswordHash` halaman;
  ditangani lewat task follow-up terpisah.
