# Runbook: penyiapan router MikroTik untuk isolir PPPoE

Dokumen ini adalah hasil verifikasi lab Task 8 (fase 1, fondasi router) dan menjadi
acuan implementasi layanan penyiapan (Task 9). Spec yang mengikat:
`docs/superpowers/specs/2026-10-29-mikrotik-pppoe-isolir-design.md` (bagian 7, 8, 11).

Setiap butir diberi salah satu label:

- **terbukti di lab (7.8), assertion otomatis**: helper lab menjalankannya dan test gagal bila hasilnya beda.
- **teramati di log lab (7.8), bukan assertion otomatis**: helper menjalankannya dan hasilnya dibaca dari log
  `t.Logf`; test tidak akan gagal bila hasilnya beda.
- **inspeksi read-only (7.8)**: kesimpulan dari membaca konfigurasi router, tanpa trafik nyata.
- **belum terbukti**: tidak dijalankan; jangan dianggap benar sampai diuji.

Tanggal uji: 30 Okt 2026. Helper uji: `internal/platform/mikrotik/isolir_lab_test.go`
(build tag `mikrotiklab`, kredensial hanya dari env). Router lab: CHR x86_64, RouterOS `7.8 (stable)`,
diakses lewat IP WireGuard.

## 1. Router dan transport yang diuji

| Butir | Hasil | Status |
|---|---|---|
| Versi router | `Version()` = `7.8 (stable)` | terbukti di lab (7.8), assertion otomatis (versi tidak kosong) |
| Binary API (TCP 8728): `Version`, `Print`, `Add`, `Set`, `Remove` | profile `zyad-labtest` dibuat, di-`Set`, dibaca ulang, dihapus, dan dipastikan hilang | terbukti di lab (7.8), assertion otomatis |
| REST port 80 (`www`) | `GET /rest/system/resource` membalas 404 | teramati di log lab (7.8), bukan assertion otomatis |
| REST port 443 (`www-ssl`) | connection refused (`www-ssl` disabled di router lab) | teramati di log lab (7.8), bukan assertion otomatis |
| `Detect` dengan `Port=0` | REST dulu (404) → fallback binary: `api_type=binary`, ±1,1 dtk | teramati di log lab (7.8), bukan assertion otomatis |
| `Detect` dengan `Port=8728` | binary dulu: `api_type=binary`, ±1,1 dtk | teramati di log lab (7.8), bukan assertion otomatis |
| Transport REST secara fungsional (v7) | tidak bisa diuji: `www-ssl` mati dan `/ip service` tidak boleh diubah di router lab | **belum terbukti** |
| RouterOS v6 (transport apa pun) | tidak ada router v6 di lab | **belum terbukti** |
| Binary API TLS (8729) | TCP 8729 terbuka menurut probe TCP koordinator dari shell sebelum Task 8 (bukan oleh helper); `/ip service` mencatat `api-ssl` aktif (log discovery); koneksi TLS API tidak pernah dicoba | **belum terbukti** |

Catatan: dokumentasi MikroTik menyebut REST lewat HTTP (`www`) baru tersedia mulai 7.9, sebelumnya hanya
lewat `www-ssl`. Ini konsisten dengan 404 di 7.8 tetapi tidak diverifikasi lebih jauh. Akibatnya router
v7 < 7.9 tanpa `www-ssl` terdeteksi sebagai `binary`; itu benar karena binary API berfungsi di v7.

## 2. Gambaran router lab (inspeksi read-only, tanpa rahasia)

- 10 PPP profile, 85 PPP secret (semua `service=pppoe`), **0 sesi aktif** sepanjang uji.
- Tidak ada secret yang punya `remote-address`; alamat sesi datang dari profile
  (`local-address` + `remote-address`=nama pool). Ini dasar desain profile isolir (4.2).
- 13 PPPoE server (sebagian disabled), 1 hotspot server.
- Filter 17 aturan. Rantai `forward`: 2 jump hotspot, fasttrack dan accept `connection-state` (keduanya
  disabled), 2 drop milik pemilik. Rantai `input`: jump hotspot (`hotspot=from-client`), drop tcp 64872-64875
  (`hotspot=!from-client`), accept udp 9993. Tabel `raw` kosong. NAT 21 aturan.
- `/ip proxy` **disabled**, 6 aturan `/ip proxy access` milik pemilik. DNS router `allow-remote-requests=true`.
- Router ini punya mekanisme suspend buatan pemilik (address-list + dst-nat + proxy access). Penyiapan zyad
  tidak menyentuhnya dan memakai nama berbeda. Sebelum uji tidak ada objek `zyad-*`.

## 3. Prinsip penyiapan

1. Semua objek yang dibuat bernama `zyad-*` (bila path punya `name`/`list`) dan ber-`comment=zyad-managed`.
2. Penyiapan **tidak pernah** mengubah, memindah, menonaktifkan, atau menghapus objek yang bukan buatannya.
3. Idempoten lewat baca-sebelum-tulis dengan kunci per tipe objek di **bagian 5**. Bila objek zyad sudah ada
   dengan atribut sama, lewati; bila beda, `Set` objek zyad itu (tanpa `place-before`). Bila nama
   pool/profile sudah ada **tanpa** `comment=zyad-managed`, penyiapan **menolak dan melapor**; tidak pernah
   mengadopsi atau mengubahnya.
4. Pembersihan hanya menghapus objek ber-`comment=zyad-managed`, urutan terbalik dari pembuatan.
5. Normalisasi `disabled`: ditulis `yes`/`no`; dibaca balik bentuknya berbeda per path. Pada 7.8 (inspeksi
   read-only lewat binary API): `/ip/pool` dan `/ppp/profile` tidak mengirim key `disabled` sama sekali;
   aturan filter/NAT yang aktif **tidak mengirim key** `disabled` (hanya yang nonaktif mengirim `true`);
   `/ppp/secret`, `/ip/firewall/address-list`, `/ip/proxy/access` mengirim `false`/`true`. Saat membandingkan:
   key tidak ada, `""`, `false`, `no` = aktif; `true`, `yes` = nonaktif. REST belum diperiksa (**belum terbukti**).

## 4. Daftar objek final (urutan pembuatan)

### Placeholder

| Placeholder | Arti | Contoh lab |
|---|---|---|
| `{ISOLIR_PROFILE}` | nama profile isolir | `zyad-isolir` |
| `{ISOLIR_POOL_RANGE}` | rentang alamat sesi terisolir, dipilih tenant, tidak boleh tumpang tindih pool/alamat lain | `10.255.254.2-10.255.254.254` |
| `{ISOLIR_SUBNET}` | prefix CIDR yang mencakup `{ISOLIR_POOL_RANGE}` (untuk `src-address` proxy access) | `10.255.254.0/24` |
| `{ISOLIR_LOCAL_ADDRESS}` | alamat sisi router untuk sesi isolir, di dalam `{ISOLIR_SUBNET}` tetapi di luar pool | `10.255.254.1` |
| `{ISOLIR_RATE_LIMIT}` | batas kecepatan profile isolir (opsional; hilangkan atributnya bila kosong) | `512k/512k` |
| `{PLATFORM_HOST}` | host platform (FQDN) | `zyad.online` (DEV) / `zyad.cloud` (prod) |
| `{TENANT_ISOLIR_HOST}` | host halaman isolir tenant (subdomain/domain tenant) | per tenant |
| `{DOKU_HOSTS}` | daftar domain pembayaran DOKU (bagian 7); satu entri address-list per elemen `{DOKU_HOSTS}[i]` | `sandbox.doku.com` |
| `{FIRST_FORWARD_ID}` | `.id` aturan pertama di rantai `forward` yang **tidak** ber-`comment=zyad-managed`, dibaca saat penyiapan (tidak diubah); kosong bila tidak ada | `*12` |
| `{REDIRECT_IP}` | IP host pengalih eksternal untuk opsi 4.5A | — |
| `{SECRET}` | nama PPP secret yang diisolir/dibuka | — |
| `{ORIGINAL_PROFILE}` | profile asal secret yang disimpan sebelum isolir; tidak boleh sama dengan `{ISOLIR_PROFILE}` | — |

### 4.1 Pool alamat isolir

```
/ip pool add name=zyad-isolir-pool ranges={ISOLIR_POOL_RANGE} comment=zyad-managed
```

- Verifikasi: `/ip pool print where name=zyad-isolir-pool` → `ranges` dan `comment` sama.
- Bersihkan: `/ip pool remove [find name=zyad-isolir-pool comment=zyad-managed]` (setelah profile dihapus).
- Sebelum menulis, baca `/ip/pool` (dan sebaiknya `/ip/address`), tolak rentang yang tumpang tindih.
- Status: Add dan Remove tanpa galat, Print setelah Add berisi pool: teramati di log lab (7.8), bukan assertion
  otomatis. Tidak tersisa setelah cleanup: terbukti di lab (7.8), assertion otomatis (`TestLabIsolirLeftovers`).
  Pemberian alamat ke sesi nyata: **belum terbukti**.

### 4.2 Profile PPP isolir

```
/ppp profile add name={ISOLIR_PROFILE} address-list=zyad-isolir \
    local-address={ISOLIR_LOCAL_ADDRESS} remote-address=zyad-isolir-pool \
    dns-server={ISOLIR_LOCAL_ADDRESS} rate-limit={ISOLIR_RATE_LIMIT} only-one=yes \
    comment=zyad-managed
```

- Atribut `address-list`, `local-address`, `remote-address`, `dns-server`, `rate-limit`, `only-one`, `comment`
  diterima dan dibaca balik sama: teramati di log lab (7.8), bukan assertion otomatis.
- `local-address` + `remote-address` diperlukan karena di router lab alamat sesi hanya datang dari profile
  (inspeksi read-only (7.8)). Perilaku sesi bila profile tanpa pool: **belum terbukti**.
- Secret yang punya `remote-address` sendiri menimpa pool profile. Akibatnya sesi itu tidak berada di
  `{ISOLIR_SUBNET}`: pencocokan proxy access berbasis subnet (4.5B) tidak kena, tetapi filter forward berbasis
  `address-list=zyad-isolir` (4.4) tetap berlaku karena keanggotaan list datang dari profile. (**belum terbukti**;
  di router lab tidak ada secret seperti itu)
- **DNS.** Ada dua jalur resolver:
  1. Klien memakai DNS router (`dns-server={ISOLIR_LOCAL_ADDRESS}`, router `allow-remote-requests=yes`): paket
     menuju router sendiri lewat rantai `input`, jadi tidak tersentuh filter forward zyad. Inspeksi read-only
     (7.8) atas rantai `input` dan tabel `raw`: tidak ada aturan yang men-drop udp/tcp 53 dari
     `10.255.254.0/24` (aturan input yang ada hanya jump hotspot `from-client`, drop tcp 64872-64875, accept udp 9993;
     `raw` kosong; policy default accept). Kesimpulan: DNS ke router **tidak diblok oleh konfigurasi lab saat ini**,
     tetapi tanpa sesi nyata ini **belum terbukti**. Router tenant lain bisa punya rantai `input` yang men-drop DNS
     dari subnet tak dikenal; penyiapan sebaiknya memeriksa dan memperingatkan, bukan mengubah aturan input.
  2. Klien memakai resolver eksternal (mis. CPE dengan DNS statis 8.8.8.8): paket lewat rantai `forward` dan
     diizinkan oleh dua aturan accept port 53 di 4.4.
- Verifikasi: `/ppp profile print where name={ISOLIR_PROFILE}`, bandingkan atribut di atas.
- Bersihkan: kembalikan dulu semua secret yang masih memakai profile ini, lalu
  `/ppp profile remove [find name={ISOLIR_PROFILE} comment=zyad-managed]`. Perilaku RouterOS bila profile masih
  dipakai secret: **belum terbukti**.

### 4.3 Address-list allowlist (FQDN)

```
/ip firewall address-list add list=zyad-isolir-allow address={PLATFORM_HOST} comment=zyad-managed
/ip firewall address-list add list=zyad-isolir-allow address={TENANT_ISOLIR_HOST} comment=zyad-managed
/ip firewall address-list add list=zyad-isolir-allow address={DOKU_HOSTS}[i] comment=zyad-managed   # satu per host
```

- Teramati di log lab (7.8), bukan assertion otomatis: entri FQDN tetap statis (`dynamic=false`, comment
  `zyad-managed`), dan dalam ≤5 dtk muncul entri `dynamic=true` berisi IP hasil resolve dengan
  **comment = nama FQDN** (`zyad.online` → 1 IP, `api-sandbox.doku.com` → 2 IP, `sandbox.doku.com` → 1 IP).
- Bersihkan: hapus entri statis ber-`comment=zyad-managed`; entri dinamisnya ikut hilang. Tidak ada entri
  `zyad-*` tersisa: terbukti di lab (7.8), assertion otomatis (`TestLabIsolirLeftovers` memeriksa juga `list=zyad-*`).
- Akurasi saat IP CDN berganti: **belum terbukti**.

### 4.4 Filter forward isolir (4 aturan, urutan penting)

```
/ip firewall filter add chain=forward src-address-list=zyad-isolir protocol=udp dst-port=53 action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir protocol=tcp dst-port=53 action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir dst-address-list=zyad-isolir-allow action=accept place-before={FIRST_FORWARD_ID} comment=zyad-managed
/ip firewall filter add chain=forward src-address-list=zyad-isolir action=drop place-before={FIRST_FORWARD_ID} comment=zyad-managed
```

- Menambah satu per satu dengan `place-before` ke id yang sama menghasilkan urutan sesuai urutan `Add`.
  Urutan lab: `[zyad accept dns udp, zyad accept dns tcp, zyad accept allowlist, zyad drop, *12, *13, ...]`.
  Teramati di log lab (7.8), bukan assertion otomatis. Aturan lama tidak diubah; hanya nomor urut tampilannya bergeser.
- Bila `{FIRST_FORWARD_ID}` kosong (rantai `forward` tanpa aturan non-zyad), tambahkan tanpa `place-before`.
- Kenapa di depan: aturan accept umum (`accept established,related`, accept-all) di depan drop zyad membuat drop
  tidak pernah kena. **FastTrack**: di router yang mengaktifkan `fasttrack-connection`, aturan zyad wajib berada
  sebelum aturan fasttrack, karena koneksi yang sudah di-fasttrack melewati filter. (Di lab fasttrack disabled;
  dampaknya **belum terbukti**.)
- Semua aturan mencocokkan `src-address-list=zyad-isolir` saja, jadi tidak menyentuh trafik non-anggota.
- Efek pemblokiran pada trafik nyata: **belum terbukti** (tidak ada sesi).

### 4.5 Pengalihan HTTP (opsional, best-effort)

**Keduanya belum terbukti end-to-end.** Yang teramati hanya bahwa router menerima objeknya.

**A. dst-nat ke host pengalih eksternal**

```
/ip firewall nat add chain=dstnat src-address-list=zyad-isolir dst-address-list=!zyad-isolir-allow \
    protocol=tcp dst-port=80 action=dst-nat to-addresses={REDIRECT_IP} to-ports=80 comment=zyad-managed
```

- Router menerima aturan termasuk negasi `!zyad-isolir-allow`: teramati di log lab (7.8), bukan assertion otomatis.
- Kelemahan: `to-addresses` harus IP, dan header `Host` tetap situs asli, sehingga server tujuan **tidak bisa
  mengenali tenant dari host** (bertentangan dengan spec bagian 11). Tidak direkomendasikan untuk Task 9.

**B. web-proxy router dengan aksi redirect**

```
/ip firewall nat add chain=dstnat src-address-list=zyad-isolir dst-address-list=!zyad-isolir-allow \
    protocol=tcp dst-port=80 action=redirect to-ports=8080 comment=zyad-managed
/ip proxy access add src-address={ISOLIR_SUBNET} dst-host={PLATFORM_HOST} action=allow comment=zyad-managed
/ip proxy access add src-address={ISOLIR_SUBNET} action=redirect action-data={TENANT_ISOLIR_HOST}/isolir comment=zyad-managed   # v7
# v6 (belum terbukti): /ip proxy access add src-address={ISOLIR_SUBNET} action=deny redirect-to={TENANT_ISOLIR_HOST}/isolir comment=zyad-managed
```

- Teramati di log lab (7.8), bukan assertion otomatis: `action=redirect action-data=...` diterima; `redirect-to`
  ditolak dengan "unknown parameter redirect-to".
- `/ip proxy access` tidak punya pencocok address-list, jadi sumber dibatasi dengan `{ISOLIR_SUBNET}` (alasan lain
  pool khusus di 4.1; lihat juga catatan secret ber-`remote-address` di 4.2).
- Syarat: `/ip proxy set enabled=yes`, setelan **global router, bukan objek zyad**. Ini membuka port proxy untuk
  semua klien dan mengaktifkan proxy access lama milik tenant. Di lab proxy **tidak** diaktifkan. Aturan `input`
  yang menolak port proxy dari selain `zyad-isolir` adalah ide mitigasi: **belum terbukti, jangan otomatis**.

**Rekomendasi untuk Task 9**: penyiapan default **tidak** membuat objek pengalihan. Isolir sah tanpa redirect
(spec bagian 11): profile isolir + filter allowlist memutus akses, dan pelanggan menjangkau pembayaran lewat
tautan di notifikasi WhatsApp/email dan halaman isolir yang dibuka langsung. Opsi B boleh menjadi langkah opsional
ber-persetujuan setelah terbukti dengan CPE nyata. HTTPS tidak bisa dialihkan bersih di kedua opsi.

## 5. Kunci pencocokan dan idempotensi (untuk Task 9)

Aturan umum: `Print` path terkait, saring `comment=zyad-managed`, cocokkan dengan tuple di bawah. Bandingkan
nilai setelah normalisasi (`disabled` per 3.5; nilai kosong = atribut tidak diset). `place-before` hanya untuk
`Add` dan **tidak boleh** ada di atribut `Set` (bukan atribut yang tersimpan, dan `Set` tidak bisa memindah urutan).

| Objek | Kunci pencocokan (Print) | Dibandingkan setelah ketemu | Bila ada tanpa `comment=zyad-managed` |
|---|---|---|---|
| Pool | `name=zyad-isolir-pool` | `ranges` | tolak dan lapor |
| Profile | `name={ISOLIR_PROFILE}` | `address-list`, `local-address`, `remote-address`, `dns-server`, `rate-limit`, `only-one` | tolak dan lapor |
| Address-list | `list=zyad-isolir-allow` + `address` (FQDN atau IP) + `comment=zyad-managed`; **abaikan baris `dynamic=true`** (comment-nya FQDN) | `disabled` | entri statis non-zyad dengan list sama: tolak dan lapor |
| Filter / NAT / proxy access | tuple di bawah + `comment=zyad-managed` | `disabled`, nilai tujuan (`to-addresses`, `to-ports`, `action-data`) | tidak berlaku (aturan non-zyad tidak pernah dicocokkan) |

Tuple aturan tanpa nama (semua atribut lain harus kosong):

| # | Path | chain | action | src-address-list | dst-address-list | protocol | dst-port | lainnya |
|---|---|---|---|---|---|---|---|---|
| F1 | `/ip/firewall/filter` | forward | accept | zyad-isolir | — | udp | 53 | — |
| F2 | `/ip/firewall/filter` | forward | accept | zyad-isolir | — | tcp | 53 | — |
| F3 | `/ip/firewall/filter` | forward | accept | zyad-isolir | zyad-isolir-allow | — | — | — |
| F4 | `/ip/firewall/filter` | forward | drop | zyad-isolir | — | — | — | — |
| NA | `/ip/firewall/nat` (opsi 4.5A) | dstnat | dst-nat | zyad-isolir | !zyad-isolir-allow | tcp | 80 | `to-addresses={REDIRECT_IP}`, `to-ports=80` |
| NB | `/ip/firewall/nat` (opsi 4.5B) | dstnat | redirect | zyad-isolir | !zyad-isolir-allow | tcp | 80 | `to-ports=8080` |
| PA | `/ip/proxy/access` (opsi 4.5B) | — | allow | — | — | — | — | `src-address={ISOLIR_SUBNET}`, `dst-host={PLATFORM_HOST}` |
| PR | `/ip/proxy/access` (opsi 4.5B) | — | redirect | — | — | — | — | `src-address={ISOLIR_SUBNET}`, `action-data={TENANT_ISOLIR_HOST}/isolir` (pada `Set`, kunci = `src-address`+`action`) |

Urutan filter F1–F4 sebagian ada:

1. Baca rantai `forward` berurutan. Ambil posisi aturan F1..F4 yang ada dan `{FIRST_FORWARD_ID}`.
2. Bila F yang ada urutannya relatif salah (mis. F4 sebelum F3), atau ada aturan non-zyad di antara mereka:
   **laporkan dan minta bersihkan + terapkan ulang**; jangan coba memperbaiki dengan `Set` (tidak bisa memindah).
3. Selain itu, buat yang hilang dengan `place-before` = `.id` aturan zyad berikutnya yang sudah ada (mis. F3 hilang
   → `place-before=<id F4>`), atau `{FIRST_FORWARD_ID}` bila tidak ada aturan zyad sesudahnya, atau tanpa
   `place-before` bila keduanya tidak ada.
4. Setelah itu verifikasi ulang: F1..F4 berurutan dan semuanya sebelum `{FIRST_FORWARD_ID}`. Bila ada aturan
   non-zyad yang berada sebelum F1 (mis. tenant menambah aturan di atas setelah penyiapan), laporkan sebagai
   "urutan perlu diterapkan ulang".

Logika ini disusun dari perilaku `place-before` yang teramati (4.4); kasus "sebagian ada" sendiri **belum terbukti** di lab.

## 6. Isolir dan buka-isolir per secret

```
# isolir
/ppp secret set [find name={SECRET}] profile={ISOLIR_PROFILE}
/ppp active remove [find name={SECRET}]
# buka-isolir
/ppp secret set [find name={SECRET}] profile={ORIGINAL_PROFILE}
/ppp active remove [find name={SECRET}]
```

- `Set profile` ke profile isolir lalu kembali ke `default` pada secret uji `zyad-labtest-user` (disabled):
  nilai `profile` berubah sesuai. Teramati di log lab (7.8), bukan assertion otomatis.
- `/ppp/active print ?name=zyad-labtest-user` mengembalikan 0 baris tanpa galat: teramati di log lab (7.8).
  Worker mencari id sesi lewat `Print` dulu dan hanya memanggil `Remove` bila ada; "tidak ada sesi" = sukses.
  Galat `Remove` pada id yang tidak ada: **belum terbukti** (helper tidak lagi menguji ini, lihat bagian 11).
- Sesi lama terputus, sesi baru mendapat profile isolir, address-list `zyad-isolir` terisi dinamis: **belum terbukti**.
- `{ORIGINAL_PROFILE}` tidak boleh `{ISOLIR_PROFILE}` (spec bagian 11).

## 7. Domain pembayaran DOKU

- API: `api-sandbox.doku.com` (sandbox), `api.doku.com` (produksi) di `internal/platform/doku/client.go`. Dipanggil
  server, bukan browser pelanggan; tidak wajib di allowlist.
- Halaman checkout: `sandbox.doku.com/checkout/link/...` hanya muncul di fixture test
  (`internal/platform/doku/client_test.go`); host checkout produksi tidak tertulis di kode.

Daftar final `{DOKU_HOSTS}` (halaman checkout, aset, CDN, halaman bank/e-wallet): **perlu uji checkout nyata**.
Awal yang masuk akal: DEV `sandbox.doku.com`; produksi host dari `PaymentURL` yang dikembalikan DOKU. Simpan
sebagai konfigurasi, bukan hardcode.

## 8. Perbedaan v6 dan v7 yang relevan

Pendaftaran router v6: isi `port` dengan port API (`8728`, atau `8729` untuk API-SSL). Deteksi transport mencoba REST
dan binary pada port yang sama (binary dulu di 8728/8729), dan lapisan HTTP mewajibkan `port` >= 1, sehingga router v6
yang didaftarkan dengan port 80/443 tidak akan terdeteksi.

| Butir | v6 | v7 | Status |
|---|---|---|---|
| REST | tidak ada | ada (`www-ssl`; `www` mulai 7.9 menurut dokumentasi) | 7.8: 404 lewat `www` teramati di log; sisanya belum terbukti |
| Binary API | ada | ada | v7 terbukti (assertion); v6 belum terbukti |
| Proxy access redirect | `action=deny redirect-to=URL` (dokumentasi) | `action=redirect action-data=URL`; `redirect-to` ditolak | v7 teramati di log; v6 belum terbukti |
| `address-list` di PPP profile | ada (dokumentasi) | ada | v7 teramati di log; v6 belum terbukti |
| FQDN di address-list | mulai 6.36 (dokumentasi) | ada | v7 teramati di log; v6 belum terbukti |
| `place-before` di filter | ada (dokumentasi) | ada | v7 teramati di log; v6 belum terbukti |
| Bentuk `disabled` saat dibaca | tidak diperiksa | lihat 3.5 | v7 binary inspeksi read-only; v6/REST belum terbukti |

## 9. Policy user API

Uji lab memakai user grup `full`. Minimum menurut operasi yang dipakai:

- Binary API: `api`, `read`, `write`.
- REST: `rest-api`, `read`, `write` (plus service `www-ssl`/`www` aktif).
- `test`, `sensitive`, `policy`, `ftp` tidak dibutuhkan operasi di atas. Bila penyiapan kelak menyimpan backup,
  periksa ulang policy-nya.

Status: **belum terbukti** dengan user berhak minimum (membuat user/grup di router lab tidak diizinkan).

## 10. Daftar "belum terbukti" (wajib diuji dengan CPE nyata sebelum fase 3 aktif di produksi)

1. Sesi PPPoE dengan profile isolir mendapat IP dari `zyad-isolir-pool` dan masuk address-list `zyad-isolir`.
2. DNS dari sesi terisolir: ke resolver router (rantai `input`, hanya inspeksi read-only) dan ke resolver eksternal (F1/F2).
3. Sesi terisolir hanya bisa DNS + host allowlist; trafik lain ter-drop (termasuk pada router dengan FastTrack aktif).
4. Halaman platform/tenant dan checkout DOKU bisa dibuka dari sesi terisolir; daftar `{DOKU_HOSTS}` final.
5. `Set profile` + `/ppp active remove` memutus sesi lama dan CPE tersambung ulang dengan profile baru; begitu juga restore.
6. Galat `Remove` pada id sesi yang tidak ada.
7. Secret dengan `remote-address` sendiri: tetap terisolir lewat address-list.
8. Pengalihan HTTP (opsi A atau B) mengantar browser ke halaman isolir tenant.
9. Semua hal di atas pada RouterOS v6 (binary API) dan v7 lewat REST; binary API TLS (8729).
   Selama REST belum terbukti, backend menolak setup `apply`/`cleanup` untuk router `api_type=rest`
   (422 `MIKROTIK_SETUP_REST_UNVERIFIED`) kecuali env `MIKROTIK_SETUP_ALLOW_REST=true`; `preview` tetap jalan.
10. User API berhak minimum cukup untuk penyiapan, isolir, restore, dan pembersihan.
11. Hapus profile isolir yang masih dipakai secret.
12. Penyiapan idempoten dengan kondisi "sebagian ada" (bagian 5).
13. Router dengan FastTrack aktif: RouterOS menambah aturan dinamis "special dummy rule" di puncak daftar filter;
    penyiapan mengabaikan baris `dynamic=true` saat memilih `{FIRST_FORWARD_ID}` dan saat memeriksa urutan. Hanya diuji
    dengan fake client; **belum terbukti di router asli** (FastTrack disabled di lab).

## 11. Jejak lab

- Backup sebelum uji: `pre-zyad-isolir-20261030.backup` di router lab (tidak dihapus). `pre-mirror-prod.backup` tidak disentuh.
- Objek uji yang dibuat lalu dihapus: pool `zyad-isolir-pool`, profile `zyad-labtest` dan `zyad-isolir`, 4 entri
  `zyad-isolir-allow`, 4 aturan filter, 2 aturan NAT (satu disabled), 2 aturan proxy access, secret `zyad-labtest-user` (disabled).
- Versi pertama helper sekali memanggil `Remove("/ppp/active", "*FFFFFF")` (id hard-coded, bukan objek buatan helper)
  saat router punya 0 sesi aktif; hasilnya galat "no such item", tidak ada sesi terdampak. Panggilan itu sudah dihapus:
  helper kini hanya `Add`/`Set`/`Remove` objek yang ia buat sendiri dengan `.id` tercatat.
- Setelah pembersihan: 0 objek `zyad-*` (assertion otomatis); jumlah profile, secret, aturan filter/NAT/mangle,
  address-list, dan proxy access sama dengan sebelum uji (log discovery); `/ip proxy` tetap disabled; 0 sesi aktif.
- Menjalankan ulang helper (backup dulu; nama backup harus baru, helper menolak menimpa):

```
set -a; . ~/.config/zyad-lab.env; set +a
go test -tags mikrotiklab -run 'TestLabIsolirDiscover|TestLabIsolirAddressing|TestLabIsolirLeftovers' -v ./internal/platform/mikrotik/
MIKROTIK_LAB_WRITE=1 MIKROTIK_LAB_BACKUP_NAME=pre-zyad-isolir-$(date +%Y%m%d%H%M) \
  go test -tags mikrotiklab -run 'TestLabIsolirBackup' -v ./internal/platform/mikrotik/
MIKROTIK_LAB_WRITE=1 go test -tags mikrotiklab -run 'TestLabIsolirTransports|TestLabIsolirMechanism' -v ./internal/platform/mikrotik/
go test -tags mikrotiklab -run 'TestLabIsolirLeftovers' -v ./internal/platform/mikrotik/
```

## 12. Operasional: timeout HTTP dan nginx

Satu siklus lab preview + apply + preview + apply + cleanup makan ±70 dtk karena klien binary login ulang per
panggilan. Handler mikrotik memperpanjang write deadline koneksi (apply/cleanup 4 menit, preview 90 dtk,
registrasi/PATCH/check 45 dtk), tetapi reverse proxy punya batasnya sendiri: di DEV/prod set
`proxy_read_timeout 300s;` untuk `location /api/v1/app/mikrotik/`. Tanpa itu klien bisa melihat 502 sementara
operasi di backend tetap selesai. Detail: `docs/reference-mikrotik.md` bagian "Timeout dan nginx".
