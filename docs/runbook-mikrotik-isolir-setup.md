# Runbook: penyiapan router MikroTik untuk isolir PPPoE

Dokumen ini adalah hasil verifikasi lab Task 8 (fase 1, fondasi router) dan menjadi
acuan implementasi layanan penyiapan (Task 9). Spec yang mengikat:
`docs/superpowers/specs/2026-10-29-mikrotik-pppoe-isolir-design.md` (bagian 7, 8, 11).

Setiap butir diberi label:

- **terbukti di lab (7.8)**: benar-benar dijalankan terhadap router lab dan hasilnya diamati.
- **belum terbukti**: tidak bisa atau belum dijalankan; jangan dianggap benar sampai diuji.

Tanggal uji: 30 Okt 2026. Helper uji: `internal/platform/mikrotik/isolir_lab_test.go`
(build tag `mikrotiklab`, kredensial hanya dari env).

## 1. Router dan transport yang diuji

| Butir | Hasil | Status |
|---|---|---|
| Router lab | CHR x86_64, RouterOS `7.8 (stable)`, diakses lewat IP WireGuard | terbukti di lab (7.8) |
| Klien binary API (TCP 8728) | `Version`, `Print`, `Add`, `Set`, `Remove` lulus | terbukti di lab (7.8) |
| Klien REST port 80 (`www`) | `GET /rest/system/resource` membalas 404 | terbukti di lab (7.8) |
| Klien REST port 443 (`www-ssl`) | koneksi ditolak (service `www-ssl` disabled di router lab) | terbukti di lab (7.8) |
| `Detect` dengan `Port=0` | REST dicoba dulu (404), lalu fallback ke binary: `api_type=binary`, ±1,1 dtk | terbukti di lab (7.8) |
| `Detect` dengan `Port=8728` | binary dicoba dulu: `api_type=binary`, ±1,1 dtk | terbukti di lab (7.8) |
| Transport REST secara fungsional (v7) | tidak bisa diuji: `www-ssl` mati dan mengubah `/ip service` dilarang di router lab | **belum terbukti** |
| RouterOS v6 (transport apa pun) | tidak ada router v6 di lab | **belum terbukti** |
| Binary API TLS (8729) | port terbuka, tetapi tidak diuji | **belum terbukti** |

Catatan: dokumentasi MikroTik menyebut REST lewat HTTP (`www`) baru tersedia mulai 7.9,
sebelumnya hanya lewat `www-ssl`. Ini konsisten dengan 404 di 7.8, tetapi tidak diverifikasi
lebih jauh. Akibatnya router v7 < 7.9 tanpa `www-ssl` akan terdeteksi sebagai `binary`, dan
itu memang perilaku yang benar (binary API berfungsi penuh di v7).

## 2. Gambaran router lab (tingkat tinggi, tanpa rahasia)

- 10 PPP profile, 85 PPP secret (semua `service=pppoe`), **0 sesi aktif** saat uji.
- Semua secret **tidak** punya `remote-address`; alamat sesi datang dari profile
  (`local-address` + `remote-address`=nama pool). Ini penting untuk desain profile isolir (bagian 4.2).
- 13 PPPoE server (sebagian disabled), 1 hotspot server.
- Firewall filter 17 aturan; rantai `forward` tidak punya aturan accept-all yang aktif
  (accept `connection-state` dan fasttrack ada tetapi disabled). NAT 21 aturan (hotspot + masquerade
  + satu dst-nat isolir lama milik pemilik). Sebelum uji tidak ada objek `zyad-*` sama sekali.
- `/ip proxy` **disabled**, dengan 6 aturan `/ip proxy access` milik pemilik.
- DNS router `allow-remote-requests=true`.
- Router ini sudah punya mekanisme suspend buatan pemilik (address-list sendiri + dst-nat +
  proxy access). Penyiapan zyad tidak menyentuhnya dan tidak memakai nama yang sama.

## 3. Prinsip penyiapan

1. Semua objek yang dibuat bernama `zyad-*` (bila path punya `name`/`list`) dan ber-`comment=zyad-managed`.
2. Penyiapan **tidak pernah** mengubah, memindah, menonaktifkan, atau menghapus objek yang bukan buatannya.
3. Idempoten: sebelum `Add`, `Print` dengan filter `name`/`list`+`comment`; bila sudah ada dan atributnya sama, lewati; bila beda, `Set` objek zyad itu.
4. Pembersihan hanya menghapus objek ber-`comment=zyad-managed`, urutan terbalik dari pembuatan.
5. Pengecekan "disabled": pada 7.8 aturan firewall/NAT yang aktif dikembalikan dengan `disabled` **string kosong**
   (bukan `false`), sedangkan pool/secret mengembalikan `false`. Verifikasi harus menganggap `""` dan `"false"`
   sama-sama aktif. (terbukti di lab (7.8))

## 4. Daftar objek final (urutan pembuatan)

Placeholder:

| Placeholder | Arti | Contoh lab |
|---|---|---|
| `{ISOLIR_PROFILE}` | nama profile isolir | `zyad-isolir` |
| `{ISOLIR_POOL_RANGE}` | rentang alamat sesi terisolir, dipilih/divalidasi tenant, tidak boleh tumpang tindih pool/alamat lain | `10.255.254.2-10.255.254.254` |
| `{ISOLIR_LOCAL_ADDRESS}` | alamat sisi router untuk sesi isolir, di luar pool | `10.255.254.1` |
| `{ISOLIR_RATE_LIMIT}` | batas kecepatan profile isolir (opsional) | `512k/512k` |
| `{PLATFORM_HOST}` | host platform (FQDN) | `zyad.online` (DEV) / `zyad.cloud` (prod) |
| `{TENANT_ISOLIR_HOST}` | host halaman isolir tenant (subdomain/domain tenant) | per tenant |
| `{DOKU_HOSTS}` | domain pembayaran DOKU (bagian 6) | `sandbox.doku.com`, ... |
| `{FIRST_FORWARD_ID}` | `.id` aturan pertama di rantai `forward` saat penyiapan (dibaca, tidak diubah) | `*12` |

### 4.1 Pool alamat isolir

```
/ip pool add name=zyad-isolir-pool ranges={ISOLIR_POOL_RANGE} comment=zyad-managed
```

- Verifikasi: `/ip pool print where name=zyad-isolir-pool` → `ranges` sama.
- Bersihkan: `/ip pool remove [find name=zyad-isolir-pool comment=zyad-managed]` (setelah profile dihapus).
- Sebelum menulis, layanan wajib membaca `/ip/pool` (dan sebaiknya `/ip/address`) lalu menolak rentang yang tumpang tindih.
- Status: Add/Print/Remove **terbukti di lab (7.8)**. Pemberian alamat ke sesi nyata **belum terbukti** (tidak ada sesi).

### 4.2 Profile PPP isolir

```
/ppp profile add name={ISOLIR_PROFILE} address-list=zyad-isolir \
    local-address={ISOLIR_LOCAL_ADDRESS} remote-address=zyad-isolir-pool \
    dns-server={ISOLIR_LOCAL_ADDRESS} rate-limit={ISOLIR_RATE_LIMIT} only-one=yes \
    comment=zyad-managed
```

- Atribut `address-list` ada di 7.8 dan tersimpan apa adanya. (terbukti di lab (7.8))
- `local-address` + `remote-address` **wajib**: di router lab alamat sesi hanya datang dari profile, jadi
  profile isolir tanpa pool kemungkinan membuat sesi gagal mendapat IP. (kebutuhan disimpulkan dari data lab;
  perilaku sesi tanpa pool **belum terbukti**)
- `dns-server` mengarah ke router; DNS ke router lewat rantai `input`, bukan `forward`, sehingga tidak
  tersentuh aturan filter isolir. Router harus `allow-remote-requests=yes` (lab: ya).
- Verifikasi: `/ppp profile print where name={ISOLIR_PROFILE}` → `address-list`, `local-address`,
  `remote-address`, `dns-server`, `rate-limit`, `only-one`, `comment` sesuai.
- Bersihkan: pastikan tidak ada secret yang masih memakai profile ini (kembalikan dulu), lalu
  `/ppp profile remove [find name={ISOLIR_PROFILE} comment=zyad-managed]`.
  Perilaku RouterOS bila profile masih dipakai secret **belum terbukti**.
- Status Add/Set/Print/Remove: **terbukti di lab (7.8)**.

### 4.3 Address-list allowlist (FQDN)

Satu entri per host. RouterOS me-resolve FQDN dan menambah entri dinamis berisi IP.

```
/ip firewall address-list add list=zyad-isolir-allow address={PLATFORM_HOST} comment=zyad-managed
/ip firewall address-list add list=zyad-isolir-allow address={TENANT_ISOLIR_HOST} comment=zyad-managed
/ip firewall address-list add list=zyad-isolir-allow address={DOKU_HOST} comment=zyad-managed   # per host DOKU
```

- Teramati: entri FQDN tetap statis (`dynamic=false`, comment `zyad-managed`), lalu dalam ≤5 dtk muncul entri
  `dynamic=true` berisi IP hasil resolve dengan **comment = nama FQDN** (bukan `zyad-managed`). Contoh:
  `zyad.online` → 1 IP, `api-sandbox.doku.com` → 2 IP, `sandbox.doku.com` → 1 IP. (terbukti di lab (7.8))
- Verifikasi: cari entri statis `list=zyad-isolir-allow comment=zyad-managed`; abaikan entri dinamis saat
  mencocokkan comment. Ada entri dinamis = DNS router berhasil resolve.
- Bersihkan: hapus entri statis ber-`comment=zyad-managed`; entri dinamisnya ikut hilang.
  (terbukti di lab (7.8): setelah hapus, 0 entri `zyad-*` tersisa)
- Domain yang memakai CDN berganti IP; RouterOS me-resolve ulang mengikuti TTL. Akurasi jangka panjang **belum terbukti**.

### 4.4 Filter forward isolir (4 aturan, urutan penting)

Keempatnya disisipkan dengan `place-before={FIRST_FORWARD_ID}`. Menambah satu per satu dengan `place-before`
ke id yang sama menghasilkan urutan sesuai urutan `Add`. Aturan lama tidak diubah; hanya nomor urut tampilannya bergeser.

```
/ip firewall filter add chain=forward src-address-list=zyad-isolir protocol=udp dst-port=53 action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir protocol=tcp dst-port=53 action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir dst-address-list=zyad-isolir-allow action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir action=drop place-before={FIRST_FORWARD_ID} comment=zyad-managed
```

- Urutan hasil di lab: `[zyad accept dns udp, zyad accept dns tcp, zyad accept allowlist, zyad drop, *12, *13, ...]`. (terbukti di lab (7.8))
- Semua aturan mencocokkan `src-address-list=zyad-isolir` saja, jadi tidak menyentuh trafik non-anggota.
- Kenapa di depan: bila router punya aturan accept umum (mis. `accept established,related` atau accept-all) di
  `forward`, aturan drop yang ditaruh di belakangnya tidak akan pernah kena. Bila rantai `forward` kosong,
  tambahkan tanpa `place-before` (urutan tetap benar).
- Verifikasi: `Print` rantai `forward`, pastikan empat aturan zyad berurutan dan berada sebelum aturan non-zyad pertama.
- Bersihkan: hapus empat aturan ber-`comment=zyad-managed`.
- Efek pemblokiran pada trafik nyata dari sesi terisolir: **belum terbukti** (tidak ada CPE/sesi).

### 4.5 Pengalihan HTTP (opsional, best-effort)

Dua mekanisme dievaluasi. **Keduanya belum terbukti end-to-end.** Yang terbukti hanya bahwa router menerima objeknya.

**A. dst-nat ke host pengalih eksternal**

```
/ip firewall nat add chain=dstnat src-address-list=zyad-isolir dst-address-list=!zyad-isolir-allow \
    protocol=tcp dst-port=80 action=dst-nat to-addresses={REDIRECT_IP} to-ports=80 comment=zyad-managed
```

- Router menerima aturan, termasuk negasi `!zyad-isolir-allow`. (terbukti di lab (7.8))
- Kelemahan: `to-addresses` harus IP (bukan FQDN), dan header `Host` tetap situs asli yang diminta pelanggan.
  Server tujuan **tidak bisa mengenali tenant dari host**, padahal spec bagian 11 menetapkan tenant dikenali dari
  host URL redirect. Butuh IP pengalih khusus per tenant atau pemetaan IP publik router → tenant. Tidak direkomendasikan untuk Task 9.

**B. web-proxy router dengan aksi redirect**

```
/ip firewall nat add chain=dstnat src-address-list=zyad-isolir dst-address-list=!zyad-isolir-allow \
    protocol=tcp dst-port=80 action=redirect to-ports=8080 comment=zyad-managed
/ip proxy access add src-address={ISOLIR_SUBNET} dst-host={PLATFORM_HOST} action=allow comment=zyad-managed
/ip proxy access add src-address={ISOLIR_SUBNET} action=redirect action-data={TENANT_ISOLIR_HOST}/isolir comment=zyad-managed   # v7
# v6: /ip proxy access add src-address={ISOLIR_SUBNET} action=deny redirect-to={TENANT_ISOLIR_HOST}/isolir comment=zyad-managed
```

- Router 7.8 menerima `action=redirect action-data=...` dan **menolak** `redirect-to` ("unknown parameter redirect-to"). (terbukti di lab (7.8))
- `/ip proxy access` tidak punya pencocok address-list, jadi pembatasan sumber memakai subnet pool isolir (alasan lain pool khusus di 4.1).
- URL tujuan membawa host tenant, sesuai spec bagian 11.
- Syarat: `/ip proxy set enabled=yes`. Itu **setelan global router, bukan objek zyad**: membuka port 8080 di sisi
  router untuk semua klien (risiko open proxy bila `input` tidak memblok) dan mengaktifkan aturan proxy access lama milik
  tenant. Di lab proxy **tidak** diaktifkan karena akan mengubah perilaku untuk klien yang ada. Bila dipakai, penyiapan
  wajib meminta persetujuan eksplisit tenant dan menambah aturan `input` yang menolak port proxy dari selain `zyad-isolir`.
  Status: **belum terbukti**.

**Rekomendasi untuk Task 9**: penyiapan default **tidak** membuat objek pengalihan. Isolir dianggap sah tanpa redirect
(spec bagian 11): profile isolir + filter allowlist sudah memutus akses, dan pelanggan menjangkau pembayaran lewat
tautan di notifikasi WhatsApp/email serta halaman isolir yang dibuka langsung. Opsi B boleh ditambahkan nanti sebagai
langkah opsional ber-persetujuan setelah terbukti dengan CPE nyata. HTTPS tidak bisa dialihkan bersih dalam kedua opsi.

## 5. Isolir dan buka-isolir per secret

```
# isolir
/ppp secret set [find name={SECRET}] profile={ISOLIR_PROFILE}
/ppp active remove [find name={SECRET}]
# buka-isolir
/ppp secret set [find name={SECRET}] profile={ORIGINAL_PROFILE}
/ppp active remove [find name={SECRET}]
```

- `Set profile` ke profile isolir dan kembali ke profile asal pada secret uji `zyad-labtest-user` (disabled):
  nilai `profile` berubah sesuai. (terbukti di lab (7.8))
- `/ppp/active print ?name=...` untuk secret tanpa sesi mengembalikan 0 baris; `Remove` id yang tidak ada mengembalikan
  galat router `no such item`. Worker harus memperlakukan "tidak ada sesi aktif" sebagai sukses (cari id lewat `Print`
  dulu, hapus hanya bila ada). (terbukti di lab (7.8))
- Sesi lama benar-benar terputus, sesi baru mendapat profile isolir, address-list `zyad-isolir` terisi dinamis: **belum terbukti** (tidak ada CPE).
- `original_profile` tidak boleh berisi `{ISOLIR_PROFILE}` (spec bagian 11): worker membandingkan nama sebelum menyimpan dan menolak restore ke profile isolir.

## 6. Domain pembayaran DOKU

Dari kode repo:

- API: `api-sandbox.doku.com` (sandbox), `api.doku.com` (produksi) — `internal/platform/doku/client.go`. Ini dipanggil server, bukan browser pelanggan; tidak wajib di allowlist.
- Halaman checkout yang dibuka pelanggan: `sandbox.doku.com/checkout/link/...` hanya muncul di fixture test
  (`internal/platform/doku/client_test.go`); host checkout produksi tidak tertulis di kode.

Daftar final domain yang dibutuhkan browser (halaman checkout beserta aset, CDN, dan halaman bank/e-wallet):
**perlu uji checkout nyata**. Allowlist awal yang masuk akal untuk DEV: `sandbox.doku.com`; untuk produksi: host
dari `PaymentURL` yang dikembalikan DOKU. Simpan daftar ini sebagai konfigurasi, bukan hardcode.

## 7. Perbedaan v6 dan v7 yang relevan

| Butir | v6 | v7 | Status |
|---|---|---|---|
| REST | tidak ada | ada (`www-ssl`; `www` mulai 7.9 menurut dokumentasi) | 7.8: 404 lewat `www` terbukti; sisanya belum terbukti |
| Binary API | ada | ada | v7 terbukti di lab (7.8); v6 belum terbukti |
| Proxy access redirect | `action=deny redirect-to=URL` | `action=redirect action-data=URL` (`redirect-to` ditolak) | v7 terbukti di lab (7.8); v6 belum terbukti |
| `address-list` di PPP profile | ada (dokumentasi) | ada | v7 terbukti; v6 belum terbukti |
| FQDN di address-list | ada mulai 6.36 (dokumentasi) | ada | v7 terbukti; v6 belum terbukti |
| `place-before` di filter | ada (dokumentasi) | ada | v7 terbukti; v6 belum terbukti |

## 8. Policy user API

Uji lab memakai user grup `full`. Kebutuhan minimum berdasarkan operasi yang dipakai:

- Binary API: `api`, `read`, `write`.
- REST: `rest-api`, `read`, `write` (plus service `www-ssl`/`www` aktif).
- `test` tidak dibutuhkan operasi di atas; `sensitive` tidak dibutuhkan (impor tidak perlu password secret);
  `policy`/`ftp` tidak dibutuhkan. Bila penyiapan kelak menyimpan backup (`/system/backup/save`), periksa ulang policy-nya.

Status: **belum terbukti** dengan user berhak minimum (membuat user/grup baru di router lab tidak diizinkan dalam uji ini).

## 9. Daftar "belum terbukti" (wajib diuji dengan CPE nyata sebelum fase 3 aktif di produksi)

1. Sesi PPPoE dengan profile isolir mendapat IP dari `zyad-isolir-pool` dan masuk address-list `zyad-isolir`.
2. Sesi terisolir hanya bisa DNS + host allowlist; trafik lain ter-drop.
3. Halaman platform/tenant dan checkout DOKU bisa dibuka dari sesi terisolir; daftar domain DOKU final.
4. `Set profile` + `/ppp active remove` benar-benar memutus sesi lama, CPE tersambung ulang dengan profile baru; begitu juga restore.
5. Pengalihan HTTP (opsi A atau B) mengantar browser ke halaman isolir tenant.
6. Semua hal di atas pada RouterOS v6 (binary API) dan pada v7 lewat REST.
7. User API berhak minimum cukup untuk seluruh penyiapan, isolir, restore, dan pembersihan.
8. Perilaku hapus profile isolir yang masih dipakai secret.

## 10. Jejak lab

- Backup sebelum uji: `pre-zyad-isolir-20261030.backup` di router lab (tidak dihapus). Backup lama `pre-mirror-prod.backup` tidak disentuh.
- Objek uji yang dibuat lalu dihapus: pool `zyad-isolir-pool`, profile `zyad-labtest` dan `zyad-isolir`, 4 entri
  `zyad-isolir-allow`, 4 aturan filter, 2 aturan NAT (satu disabled), 2 aturan proxy access, secret `zyad-labtest-user` (disabled).
- Setelah pembersihan: 0 objek `zyad-*`; jumlah profile, secret, aturan filter/NAT/mangle, address-list, dan proxy access sama dengan sebelum uji; `/ip proxy` tetap disabled; 0 sesi aktif sepanjang uji.
- Menjalankan ulang helper:

```
set -a; . ~/.config/zyad-lab.env; set +a
go test -tags mikrotiklab -run 'TestLabIsolirDiscover|TestLabIsolirLeftovers' -v ./internal/platform/mikrotik/
MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run 'TestLabIsolirTransports|TestLabIsolirMechanism' -v ./internal/platform/mikrotik/
```
