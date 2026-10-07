# Runbook: Cutover Halaman Marketing Baru (R5-S5)

## Tujuan

Mengganti beranda platform (`zyad.cloud/`) dari halaman lama `public-marketing` ke halaman
marketing baru (slug `beranda`, dibuat dari starter "Zyad Marketing"), tanpa deploy ulang,
dengan rollback cepat. Dokumen ini dipakai dua kali: di DEV (`zyad.online`) dahulu, lalu di
produksi setelah ada persetujuan eksplisit PO/pemilik repo.

Rujukan: spec `docs/superpowers/specs/2026-10-07-landing-marketing-v2-design.md` §9.3, §9.4, §12.

## Perilaku backend (R5-S5-T1)

- `GET /api/v1/public/landing/resolve?home=1` me-resolve beranda org dari konteks tenant
  terverifikasi (bukan dari nama di request). `home=1` menang atas `slug`.
- Beranda = halaman org dengan `is_homepage = true`, bukan template, tidak `archived`, dan
  **published**. Bila tidak ada, atau halamannya belum published / tidak ditemukan,
  fallback ke halaman slug `public-marketing`.
- Halaman `public-marketing` lama disemai migration 000035 dengan `is_homepage = true`.
- `SetHomepage` (dipanggil `Update` saat `is_homepage=true`) memakai advisory lock per org,
  melepas flag dari halaman lain secara otomatis (termasuk yang archived), lalu menyetel
  flag pada halaman target. Indeks unik `idx_landing_pages_organization_homepage_unique`
  tidak terlanggar.
- **Bukan satu transaksi dengan field lain, tetapi urutannya aman.** Di `pageService.Update`
  (setelah perbaikan review), field lain di-update **dulu**, baru `SetHomepage`. Bila update
  field gagal (mis. konflik slug 409), beranda **tidak pindah**. Target divalidasi sebelum
  keduanya: halaman `archived` ditolak **409 `PAGE_HOMEPAGE_TARGET_ARCHIVED`** (pulihkan
  halaman lebih dulu), dan halaman template (atau `is_homepage` bersama `is_template`)
  ditolak 422 `PAGE_HOMEPAGE_TEMPLATE_INVALID`. Beranda lama tetap utuh pada semua
  penolakan itu. Praktik tetap disarankan: setel `is_homepage` sebagai langkah terpisah dan
  terakhir.
- **Urutan rilis WAJIB BE dulu, baru FE.** FE baru memanggil `/resolve?home=1`; BE lama
  mengabaikan `home`, sehingga beranda platform bisa rusak pada jendela deploy bila FE naik
  lebih dulu. Konsekuensinya, rollback BE saja memutus FE baru: bila keduanya perlu
  di-rollback, **rollback FE dulu**, baru BE.
- Menyetel halaman yang masih draft sebagai beranda membuat `/` tetap jatuh ke
  `public-marketing` sampai halaman itu dipublish.
- Host tenant (custom domain) tidak terpengaruh: tetap me-resolve halaman tenant lewat
  domain binding.

## Gerbang produksi (WAJIB dibaca sebelum menyentuh produksi)

Bagian ini prasyarat sekaligus risiko, bukan perintah yang dijalankan sekarang. Setiap
langkah di produksi memerlukan **persetujuan eksplisit PO/pemilik repo**.

### a. Produksi belum menerima R4

Produksi (VPS BiznetGIO `103.87.66.189`, `zyad.cloud`) belum menerima R4. `staging-dev`
memuat R1-R5. Merilis `staging-dev` ke produksi membawa migration 000151-000155, termasuk:

- **000154** (pensiun plan/subscription/billing lama). Guard-nya menolak berjalan bila ada
  subscription berbayar aktif atau invoice billing open. Prasyarat: `accessmigrate preflight`
  bersih di produksi, `backfill-default` dijalankan, `pg_dump` arsip 9 tabel terkait sebelum
  migrate (ikuti proses R4-S5), dan data berbayar nyata dipindah manual ke contract.
- R4 S2 + S3 + S4 harus naik **bersamaan** (self-serve checkout dan entitlement).
- S5 (pricing katalog) bergantung pada R4; halaman baru tidak boleh naik tanpa R4.

### b. Header CSP nginx (S1)

Header CSP dipasang di vhost DEV dan di file repo `frontend/deploy/nginx.conf`. Produksi
memakai nginx host sendiri, jadi header **harus dipasang manual** di vhost SPA produksi,
pada semua `location /` yang melayani HTML SPA, dengan `connect-src` memuat origin API
produksi (`https://api.zyad.cloud`). Bila ragu, mulai dengan
`Content-Security-Policy-Report-Only`, lalu naikkan ke enforce setelah console bersih.
Cek inventaris origin (Google Identity, Google Fonts) agar tidak terblokir. Reload nginx
setelah `nginx -t`; rollback = hapus/komentari header lalu reload.

### c. Konfigurasi build FE baru

`VITE_LANDING_RENDERER` (default `shadow`; `iframe` = rollback renderer) harus diperhatikan
di build FE produksi. Risiko K7 diterima PO pada 7 Okt 2026: HTML tenant berada di dokumen
utama sementara token bearer disimpan di `localStorage`. Backlog: pentest dan pindah ke
cookie httpOnly.

### d. Proses deploy produksi manual

Deploy produksi dilakukan manual (struktur release + symlink di VPS; lihat catatan proses
deploy VPS yang dipegang tim). **Jangan menimpa `.env` produksi** saat menyalin release.
Jangan menulis atau menempelkan secret ke dokumen, tiket, atau log.

### e. Input PO sebelum cutover produksi

- [ ] Produk publik + harga final di Sales -> Produk (katalog publik di DEV saat ini
      KOSONG; seed dulu untuk QA).
- [ ] `featuredCode` (= `listing_code` produk) untuk kartu unggulan.
- [ ] OG image 1200x630.
- [ ] Verifikasi FAQ: metode pembayaran DOKU dan kanal support.
- [ ] Akun PIC lead.
- [ ] Keputusan menampilkan/menyembunyikan strip fakta produk (spec §9.2 butir 8).
- [ ] Cek klaim fitur §9.1 terhadap produksi.
- [ ] Kebijakan "tanpa kartu kredit" (teks yang menyebutnya harus benar).

### f. Gerbang QA sebelum produksi (belum terpenuhi)

Utang QA tercatat di ledger R5 S1-S5:

- [ ] QA berbasis login: editor, publish, form -> lead nyata, checkout DOKU sandbox.
- [ ] Lighthouse mobile: LCP < 2,5 s, CLS < 0,1. Risiko: delay/fade `zy-anim-hero`, aurora.
- [ ] Before/after visual S1.
- [ ] Animasi di browser nyata.

**Peringatan DEV:** DEV memakai kredensial email produksi. Jangan submit form/lead nyata di
DEV tanpa pengaman (mis. alamat uji milik sendiri dan PIC uji).

## Langkah cutover

Urutan sama di DEV dan produksi. Tiap langkah punya hasil yang harus dicek sebelum lanjut.

### 1. Prasyarat PO

Semua centang di bagian "Gerbang produksi / e" terpenuhi. Hasil: daftar produk publik
tampil di Sales -> Produk dengan harga final; `featuredCode` dan akun PIC diketahui.

### 2. Menu header platform

Header halaman marketing membaca menu **tenant-wide** (slot `tenant-nav`). Menu diedit di
editor: buka halaman di Landing -> Content (editor GrapesJS), pilih blok header ->
panel **Header** -> akordeon **Navigation**. Item yang dibutuhkan:

| Label | Tujuan |
|---|---|
| Produk | anchor `alur` |
| Harga | anchor `harga` |
| Konsultasi | anchor `konsultasi` |
| Masuk | `/auth/login` |

Tombol aksi header ("Mulai gratis" -> `/auth/register`) sudah dibawa starter lewat
`data-zyad-header` (akordeon "Action & Tampilan").

Perhatian: perubahan menu **langsung tersimpan tenant-wide** (tidak menunggu publish) dan
juga tampil di `public-marketing` yang masih jadi beranda. Hasil yang dicek: menu terlihat
sesuai tabel di pratinjau editor.

### 3. Buat halaman

1. Landing -> **Pages** -> buat page baru: slug `beranda`, visibilitas publik. Halaman baru
   otomatis builder GrapesJS dan langsung terbuka di editor (Content).
2. Pada dokumen kosong muncul picker "Mulai dari mana?". Pilih starter **Zyad Marketing**
   (hanya muncul untuk org platform). Picker hanya tampil untuk dokumen kosong; bila
   terlewat, buat page baru lagi.
3. Blok **Form Konsultasi** -> klik **Buat form standar**, lalu isi **PIC lead**.
4. Blok **Pricing Katalog** -> pilih **Kartu unggulan** (nilai = `listing_code` produk).
   Harga dan fitur diambil dari Sales -> Produk.

Hasil: editor memuat 12 `<section>` + 2 slot (nav/footer), form terpilih dengan PIC, kartu unggulan terisi.

### 4. SEO halaman

Landing -> Pages -> aksi SEO pada `beranda`. Isi persis:

- Meta title: `Zyad Cloud — CRM, Penawaran & Penagihan dalam Satu Platform`
- Meta description: `Kelola lead, penawaran, sales order, dan tagihan berulang di satu platform. Untuk bisnis Indonesia, dengan support lokal. Mulai gratis.`
- OG image: aset 1200x630 dari PO.

### 5. Publish dan QA singkat

Publish `beranda` (aksi Publish di daftar Pages), lalu buka `/beranda`. QA singkat: 12
`<section>` (plus slot nav/footer) tampil berurutan (1440px dan 390px), menu header menggulir ke section,
`/beranda#harga` menggulir ke harga, "Hubungi sales" mengisi Minat di form konsultasi,
animasi berjalan dan mati pada `prefers-reduced-motion`, console tanpa pelanggaran CSP.
Detail QA lengkap ada di Task 5 plan.

### 6. Cutover

Setel `beranda` sebagai beranda. Lakukan sebagai langkah **terpisah dan terakhir** (lihat
catatan non-transaksional di atas).

- **Lewat UI:** Landing -> Pages -> edit `beranda` -> centang **Set as homepage** -> simpan.
  Catatan: form ini mengirim semua field sekaligus. Jangan ubah slug/title pada penyimpanan
  yang sama; bila `page_type` bernilai `homepage`, flag selalu dikirim `true` pada setiap
  simpan.
  **PERINGATAN:** menyimpan `public-marketing` lewat form Pages (`page_type='homepage'`)
  mengirim `is_homepage=true` dan **MENGAMBIL alih beranda**, termasuk dari `beranda`.
  FE sedang diperbaiki agar tidak mengirim flag tanpa perubahan eksplisit; sampai rilis FE
  terbaru terpasang, jangan menyimpan halaman `homepage` lewat form Pages kecuali memang
  ingin memindah beranda. Halaman `archived` tidak bisa dijadikan beranda (409); pulihkan dulu.
- **Lewat API** (alternatif, token diisi sendiri, jangan ditempel ke dokumen):

  ```bash
  curl -sS -X PATCH "https://<API_HOST>/api/v1/admin/landing-pages/<BERANDA_PAGE_ID>" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"is_homepage": true}'
  ```

Hasil: buka `/` -> halaman baru tampil; `public-marketing` otomatis kehilangan flag.

### 7. Rollback

Tanpa deploy. Pilih salah satu:

- Setel `is_homepage` pada `public-marketing` (UI "Set as homepage" atau `PATCH` seperti
  di atas dengan id halaman itu); atau
- Unpublish `beranda` -> `/` jatuh ke `public-marketing` lewat fallback.

Hasil: `/` kembali menampilkan halaman lama. **PERINGATAN:** menyimpan `public-marketing`
lewat form Pages (`page_type='homepage'`) mengirim `is_homepage=true` dan MENGAMBIL beranda;
lakukan hanya bila memang bermaksud menjadikannya beranda lagi (hati-hati sampai rilis FE
terbaru yang tidak lagi mengirim flag tanpa perubahan eksplisit).

### 8. Satu rilis setelah stabil

Arsipkan `public-marketing` (aksi Archive di Pages). **Jangan diarsipkan sebelum satu rilis
stabil**, karena itu satu-satunya fallback bila `beranda` di-unpublish.

## Rollback

| Yang di-rollback | Cara | Perlu deploy? |
|---|---|---|
| Renderer (Shadow DOM) | Build FE dengan `VITE_LANDING_RENDERER=iframe`, deploy FE | Ya (build FE) |
| Beranda | Setel `is_homepage` ke `public-marketing`, atau unpublish `beranda` | Tidak |
| Fitur (kode BE/FE) | Pasang release sebelumnya (symlink release) | Ya |
| Migration 000155 (sinkron form -> CRM) | Aditif dan punya `down` (`000155_landing_form_crm_sync.down.sql`); umumnya cukup rollback kode, jalankan `down` hanya bila perlu | Migrasi |
| Header CSP | Hapus/komentari header di vhost, `nginx -t`, reload | Konfigurasi nginx |

## Verifikasi pasca-cutover

**Host untuk `curl`.** Konteks org publik diturunkan middleware `ResolvePublicOrganization`
(`internal/core/middleware/public_tenant.go`) dari header `Host`, atau `X-Forwarded-Host`
hanya bila `TrustForwardedHost` aktif **dan** peer berada di `TrustedProxyCIDRs` (mis. nginx
lokal). `PublicHostResolver` memetakan org platform hanya bila host sama dengan domain primer
platform (`zyad.cloud`; DEV `zyad.online`) atau domain `platform` terdaftar yang aktif. Host
API yang tidak terdaftar (mis. `api.zyad.cloud` dipanggil langsung tanpa `X-Forwarded-Host`
tepercaya) menghasilkan 404 `PUBLIC_HOST_NOT_FOUND`, bukan beranda platform. Karena itu
`curl` harus ke host yang dipetakan ke org platform, misalnya
`https://zyad.cloud/api/v1/public/landing/resolve?home=1` (lewat nginx yang meneruskan
`/api/`), bukan ke host API polos. Header `X-Forwarded-Host` yang dikirim klien dari luar
proxy tepercaya diabaikan.

| Cek | Perintah / cara | Hasil yang diharapkan |
|---|---|---|
| Beranda | `curl -sS -o /dev/null -w '%{http_code}\n' https://<DOMAIN>/` dan `curl -sS https://<DOMAIN>/ \| grep -i '<title'` | `200`, judul sesuai §9.3 |
| Resolve beranda | `curl -sS "https://zyad.cloud/api/v1/public/landing/resolve?home=1"` (DEV: `https://zyad.online/...`) | slug `beranda`, status published |
| Header CSP | `curl -sSI https://<DOMAIN>/ \| grep -i content-security-policy` | header ada, `connect-src` memuat origin API |
| Console | Buka `/` di browser, DevTools Console | tanpa pelanggaran CSP |
| Health | `curl -sS https://<API_HOST>/healthz` | OK |
| Host tenant | Buka `/` pada custom domain tenant | tetap halaman tenant |
