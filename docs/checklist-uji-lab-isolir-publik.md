# Checklist Uji Lab: Halaman Isolir Publik (`/isolir`)

> **Status: belum ada satu pun butir di dokumen ini yang dijalankan.** Kolom "Hasil" sengaja kosong dan harus diisi
> penguji saat uji lab nyata, apa adanya (lulus, gagal, atau sebagian, beserta bukti). Dokumen ini bukan klaim bahwa
> fitur sudah lulus uji lab.

## Tujuan

Memastikan halaman isolir publik (`/isolir`), tautan invoice `/i/:token`, dan blok tahap pemulihan benar-benar dapat
dijangkau dan berfungsi dari sesi pelanggan yang sedang terisolir oleh Mikrotik, serta menutup gerbang aktivasi di
luar kode (spec `docs/superpowers/specs/2026-11-08-mikrotik-halaman-isolir-publik-design.md` bagian 8).

## Prasyarat umum

- Setup router sudah diterapkan dengan parameter `platform_host`, `tenant_isolir_host`, `redirect_url`
  (lihat `docs/runbook-mikrotik-isolir-setup.md`, terutama bagian 4.5).
- Tenant uji memiliki entitlement `mikrotik.enabled` dan minimal satu pelanggan PPPoE yang terhubung ke invoice.
- Satu CPE nyata (atau CPE lab) yang dapat di-PPPoE ke router uji, dan satu perangkat klien di belakangnya.
- Untuk butir (d): kredensial sandbox DOKU dan satu invoice uji dengan `pay_url` aktif.
- Catat versi RouterOS, build BE/FE yang terpasang, dan tanggal uji di bagian "Lingkungan uji" di bawah.

## Lingkungan uji (diisi saat uji)

| Item | Nilai |
|---|---|
| Tanggal / penguji | |
| Versi RouterOS | |
| Build BE / FE | |
| `platform_host` | |
| `tenant_isolir_host` | |
| CPE / model | |

## Butir uji

### (a) `/isolir` terbuka dari sesi CPE terisolir lewat allowlist

- **Prasyarat:** pelanggan uji berstatus terisolir (secret masuk profil/address-list isolir); `platform_host` dan
  `tenant_isolir_host` ada di allowlist firewall isolir.
- **Langkah:**
  1. Dari klien di belakang CPE terisolir, buka `https://{tenant_isolir_host}/isolir`.
  2. Cek juga `https://{platform_host}/` untuk aset SPA (JS/CSS) yang dimuat halaman.
  3. Cari dengan username PPPoE uji, lalu dengan nomor WhatsApp uji.
  4. Dari klien yang sama, buka domain di luar allowlist (misal situs umum) sebagai kontrol negatif.
- **Hasil yang diharapkan:** halaman termuat penuh (tanpa aset gagal), pencarian mengembalikan banner isolir dan
  daftar tagihan; domain di luar allowlist tetap tertahan.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

### (b) Tautan `/i/:token` dan checkout DOKU dapat dimuat dari sesi terisolir

- **Prasyarat:** butir (a) dipahami; invoice uji punya `pay_url`.
- **Langkah:**
  1. Dari halaman `/isolir` pada sesi terisolir, klik **Bayar**.
  2. Pastikan halaman `/i/:token` termuat (termasuk PDF invoice).
  3. Lanjutkan ke checkout DOKU; amati domain apa saja yang diminta browser (DevTools Network atau log firewall).
  4. Catat domain yang tertahan oleh firewall isolir.
- **Hasil yang diharapkan:** `/i/:token` dan halaman checkout DOKU termuat sampai bisa dibayar; tidak ada domain
  checkout yang diperlukan tetapi tertahan.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

### (c) Redirect opsi B ke `{TENANT_ISOLIR_HOST}/isolir`

- **Prasyarat:** RouterOS 7 dengan web-proxy opsi B diaktifkan lewat `redirect_url`
  (runbook `docs/runbook-mikrotik-isolir-setup.md` butir 8, bagian 4.5). Fitur ini mati secara default.
- **Langkah:**
  1. Dari klien terisolir, buka situs HTTP biasa (bukan HTTPS), misal `http://neverssl.com`.
  2. Amati apakah browser dialihkan ke `{TENANT_ISOLIR_HOST}/isolir`.
  3. Ulangi dengan situs HTTPS dan catat perilakunya (pengalihan HTTPS umumnya tidak andal).
- **Hasil yang diharapkan:** pengalihan HTTP ke `/isolir` berhasil. Dokumen ini mengakui bahwa butir ini **boleh
  gagal**; bila gagal, catat apa adanya (versi RouterOS, gejala, log) dan jangan mengaktifkan opsi B di produksi.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

### (d) E2E sandbox: jatuh tempo, isolir, bayar, tahap pemulihan

- **Prasyarat:** sandbox DOKU aktif; `MIKROTIK_ISOLATION_MODE` sesuai rencana uji (catat nilainya); webhook DOKU
  sandbox menjangkau backend uji.
- **Langkah:**
  1. Terbitkan invoice uji dan biarkan lewat jatuh tempo (atau majukan tanggalnya di lingkungan uji).
  2. Verifikasi pelanggan terisolir (internet terjeda, `/isolir` menampilkan banner).
  3. Bayar lewat sandbox DOKU dari `/isolir` -> **Bayar**.
  4. Kembali ke `/i/:token?paid=1` dan amati blok tahap: `received`, `restoring`, `active`.
  5. Uji juga kasus `other_invoices` (ada tagihan menunggak lain) dan `needs_help` (misal task pemulihan sengaja
     digagalkan): pastikan tautan kontak `tel:`, `mailto:`, `wa.me` muncul dan benar.
  6. Pastikan internet pelanggan benar-benar pulih bila tahap `active`.
- **Hasil yang diharapkan:** tahap bergerak sesuai urutan, internet pulih pada `active`, dan `needs_help` hanya
  muncul dengan kontak tenant yang benar.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

### (e) nginx: tenant host melayani SPA di `/isolir`; timeout API

- **Prasyarat:** akses ke konfigurasi nginx host tenant dan host platform.
- **Langkah:**
  1. Pastikan server block `tenant_isolir_host` melayani SPA di path `/isolir` (fallback ke `index.html`).
  2. Periksa redirect HTTP ke HTTPS: dari sesi terisolir, `http://{tenant_isolir_host}/isolir` tidak boleh memutus
     allowlist (allowlist harus mencakup port 80 dan 443, atau redirect tidak melewati host di luar allowlist).
  3. Periksa host API yang dipakai SPA pada host tenant (request `/api/v1/public/isolir/*` berhasil dari host itu).
  4. Periksa `proxy_read_timeout` untuk lokasi `/api/v1/app/mikrotik/` (gerbang aktivasi spec bagian 8) dan catat
     nilainya serta apakah operasi setup/aktivasi yang lama masih selesai tanpa 504.
  5. Setelah mengubah konfigurasi: `sudo nginx -t && sudo systemctl reload nginx`; simpan salinan konfigurasi lama
     untuk rollback.
- **Hasil yang diharapkan:** `/isolir` termuat di tenant host tanpa redirect yang memutus allowlist; API publik
  terjangkau; `proxy_read_timeout` cukup untuk operasi Mikrotik terpanjang.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

### (f) Daftar domain checkout DOKU final

- **Prasyarat:** satu pembayaran sandbox DOKU dilakukan dari sesi terisolir (lihat butir (b)).
- **Langkah:**
  1. Dari log firewall/DevTools, kumpulkan semua domain yang dipakai halaman checkout DOKU (HTML, JS, gambar, API).
  2. Bandingkan dengan allowlist isolir yang terpasang; tambahkan yang kurang, ulangi pembayaran.
  3. Tetapkan daftar final dan catat sumbernya (sandbox atau produksi; domain produksi bisa berbeda dari sandbox).
- **Hasil yang diharapkan:** daftar domain final tertulis, dan pembayaran selesai dari sesi terisolir dengan allowlist
  tersebut.

| Hasil (diisi saat uji) | Bukti / catatan |
|---|---|
| | |

## Ringkasan keputusan aktivasi (diisi setelah uji)

| Butir | Lulus / Gagal / Sebagian | Tindak lanjut |
|---|---|---|
| (a) | | |
| (b) | | |
| (c) | | |
| (d) | | |
| (e) | | |
| (f) | | |
