# Referensi Modul Mikrotik (Fase 1: fondasi router)

Status: **WIP**. Spec: `docs/superpowers/specs/2026-10-29-mikrotik-pppoe-isolir-design.md`.
Runbook penyiapan router (hasil uji lab): `docs/runbook-mikrotik-isolir-setup.md`.

## Tujuan

Tenant mendaftarkan router MikroTik miliknya, menguji koneksi, lalu menyiapkan objek isolir PPPoE di router
lewat alur preview, apply, cleanup. Fase 1 hanya fondasi; belum ada isolir otomatis.

## Endpoint

Semua di bawah `/api/v1/app/mikrotik`, memakai envelope standar. Detail skema ada di `api/openapi.yaml` (tag `Mikrotik`).

| Method | Path | Permission | Fungsi |
|---|---|---|---|
| GET | `/routers` | `mikrotik_router.read` | List router (paginasi) |
| POST | `/routers` | `mikrotik_router.create` | Daftarkan router |
| GET | `/routers/{id}` | `mikrotik_router.read` | Detail router |
| PATCH | `/routers/{id}` | `mikrotik_router.update` | Update parsial |
| DELETE | `/routers/{id}` | `mikrotik_router.delete` | Hapus pendaftaran (204) |
| POST | `/routers/{id}/check` | `mikrotik_router.update` | Uji koneksi + deteksi versi |
| POST | `/routers/{id}/setup/preview` | `mikrotik_router.setup` | Rencana tindakan + `hash` (read-only) |
| POST | `/routers/{id}/setup/apply` | `mikrotik_router.setup` | Terapkan rencana (`plan_hash` wajib) |
| POST | `/routers/{id}/setup/cleanup` | `mikrotik_router.setup` | Hapus objek `zyad-managed` |

Registrasi (`POST /routers`) mewajibkan `port` >= 1 (maksimal 65535) di lapisan HTTP. **RouterOS v6** (tanpa REST)
wajib didaftarkan dengan port API: `8728`, atau `8729` untuk API-SSL. Deteksi transport mencoba REST dan binary pada
port yang **sama** (binary lebih dulu di 8728/8729), jadi port HTTP 80/443 tidak akan menemukan API binary.

List: `per_page` default 20, maksimal 200; nilai lebih besar dipotong dan `meta.per_page` memuat ukuran efektif.

Respons router tidak pernah memuat password, hanya `has_password`. Tabel: `mikrotik_routers` (migration `000168`),
permission di migration `000169`.

## Entitlement dan tenant

Seluruh grup route butuh entitlement `mikrotik.enabled` (default mati; diaktifkan manual per organization) dan
organization aktif bertipe customer atau platform.

## Aturan keamanan

- **Guard SSRF**: host di-resolve dan hanya boleh publik, atau berada di CIDR privat yang diizinkan lewat env
  `MIKROTIK_ALLOWED_PRIVATE_CIDRS` (daftar dipisah koma; kosong berarti semua privat ditolak). Pelanggaran: `MIKROTIK_HOST_NOT_ALLOWED`.
  Daftar ini dipakai bersama oleh **semua tenant**: isi dengan rentang tersempit yang hanya memuat router tenant (mis.
  subnet tunnel WireGuard router) dan kecualikan host infrastruktur (database, Redis, RADIUS, layanan lain), karena
  setiap tenant bisa mengarahkan probe router ke host/port mana pun di dalamnya. Prefix yang beririsan dengan loopback
  (`127.0.0.0/8`, `::1`), link-local (`169.254.0.0/16`, `fe80::/10`), `0.0.0.0/8`, alamat unspecified, atau multicast
  diabaikan saat config dibaca (mis. `64.0.0.0/2` ditolak karena memuat `127/8`). Loopback tidak pernah diizinkan,
  walau terdaftar.
- **TLS wajib untuk host publik** (`MIKROTIK_TLS_REQUIRED`). Sertifikat tidak tepercaya ditolak dengan
  `MIKROTIK_TLS_UNTRUSTED` yang membawa `fingerprint` dan `pin_mismatch` agar UI dapat menawarkan **pin fingerprint**
  (`tls_fingerprint`, SHA-256 leaf).
- **Password terenkripsi** di database dan tidak pernah dikembalikan; error tak dikenal tidak dibocorkan ke klien.
- **Redirect HTTP ditolak** oleh klien REST (tidak mengikuti redirect), untuk mencegah pengalihan ke host lain.
- **Batas respons**: 8 MiB per panggilan (REST dan binary), 1 MiB per word binary API.
- **Rate limit**: route yang login ke router tenant (`POST /routers`, `PATCH /routers/{id}`, `/check`, `/setup/*`)
  dibatasi 20 permintaan per menit per organization (Redis, fixed window). Lewat batas: 429 `RATE_LIMITED`. Urutan
  gerbang: tenant + entitlement, rate limit, lalu permission. Redis tidak tersedia: fail open (permintaan tetap
  dilayani, kegagalan dicatat). List, detail, dan delete tidak dibatasi.

## Semantik setup

- Input setup (`preview` dan `apply`): `platform_host` dan `pool_cidr` wajib. `pool_cidr` adalah jaringan IPv4 privat, prefix /16 sampai /29, bit host nol (mis. `10.255.254.0/24`). `profile_name` opsional, default `zyad-isolir`, harus diawali `zyad-`. `rate_limit` opsional, format seperti `512k/512k`. Host allowlist (`platform_host`, `tenant_isolir_host`, `doku_hosts`) hanya literal IPv4 atau FQDN (tanpa IPv6, skema, atau port). Pelanggaran: 422 `VALIDATION_ERROR`.
- `PATCH /routers/{id}` memprobe ulang router bila host, port, use_tls, tls_fingerprint, username, atau password berubah (bisa 502 `MIKROTIK_AUTH_FAILED`/`MIKROTIK_UNREACHABLE`); perubahan endpoint (host, port, api_type, use_tls, tls_fingerprint) mereset `isolir_ready` ke false.
- `preview` membaca router dan menghasilkan daftar tindakan plus `hash`; tidak menulis apa pun.
- `apply` menerima input yang sama plus `plan_hash`. Bila state router atau input berubah sejak preview:
  `MIKROTIK_SETUP_PLAN_CHANGED`, tidak ada yang ditulis.
- Semua objek bernama `zyad-*` dan ber-`comment=zyad-managed`. Objek bernama sama tanpa tanda itu **ditolak**
  (`MIKROTIK_SETUP_CONFLICT`), tidak pernah diadopsi atau diubah.
- **Fail-closed**: `isolir_ready` dikosongkan sebelum menulis dan baru true setelah semua langkah sukses. Kegagalan langkah
  menghasilkan `MIKROTIK_SETUP_FAILED`; bila objek berubah tetapi status gagal tersimpan: `MIKROTIK_SETUP_STATE_NOT_SAVED`
  (ulangi operasi yang sama, idempoten). Bila `isolir_ready` tidak bisa dikosongkan sebelum perubahan, router tidak
  disentuh sama sekali: `MIKROTIK_SETUP_MARK_FAILED` (aman diulang).
- `cleanup` hanya menghapus objek `zyad-managed` dan aman diulang.
- `apply` dan `cleanup` pada router yang sama diserialkan dengan advisory lock Postgres per router (try-lock, tidak
  menunggu) yang diambil sebelum state router dibaca. Bila lock sedang dipegang: 409 `MIKROTIK_SETUP_IN_PROGRESS`, tidak
  ada yang dibaca atau ditulis. `preview` tidak mengambil lock. Kunci lock memuat organisasi dan router
  (`mikrotik.setup:<org>:<router>`), jadi tenant lain tidak bisa menahan lock milik pemilik router. Karena tiap lock
  menahan satu koneksi pool sampai beberapa menit, jumlah `apply`/`cleanup` yang berjalan bersamaan dibatasi 2 per
  proses; yang melebihi batas mendapat 409 `MIKROTIK_SETUP_IN_PROGRESS` yang sama.
- **Setup lewat REST dikunci**: jalur tulis REST (PUT add dengan `place-before`, filter query, PATCH/DELETE per `.id`)
  baru diuji dengan httptest. Selama env `MIKROTIK_SETUP_ALLOW_REST` bernilai `false` (default), `apply` dan `cleanup`
  untuk router `api_type=rest` ditolak dengan 422 `MIKROTIK_SETUP_REST_UNVERIFIED` sebelum ada tulisan ke router atau
  perubahan `isolir_ready`. `preview` tetap diizinkan. Gunakan binary API (port 8728/8729) atau aktifkan flag hanya
  untuk verifikasi.
- Redirect **mati secara default**; hanya direncanakan bila `redirect_url` diisi.
- Router yang belum disiapkan tetap terdaftar (`isolir_ready=false`).

## Timeout dan nginx

- Server memakai `HTTP_WRITE_TIMEOUT_SECONDS` (default 15 dtk), padahal pekerjaan router bisa lebih lama: klien binary
  membuka satu koneksi + login per panggilan RouterOS, sehingga `apply`/`cleanup` di lab makan 15-25 dtk. Handler
  mikrotik memperpanjang write deadline koneksinya sendiri (`http.NewResponseController`): `setup/apply` dan
  `setup/cleanup` 4 menit, `setup/preview` 90 dtk, `POST /routers`, `PATCH /routers/{id}`, dan `/check` 45 dtk. Writer
  yang tidak mendukung deadline diabaikan (request tetap jalan dengan default server).
- Batas waktu sisi klien router: `Detect` (registrasi/PATCH) selesai dalam satu anggaran total maksimal 12 dtk (kecuali
  `Endpoint.Timeout` dikonfigurasi lebih besar); probe pertama maksimal 5 dtk, probe kedua mendapat sisa anggaran.
  Lookup DNS host router dibatasi 3 dtk dan kegagalannya dilaporkan sebagai `MIKROTIK_UNREACHABLE`. Satu panggilan
  binary API memakai satu deadline untuk dial + TLS + login + perintah.
- **nginx (DEV/prod)**: set `proxy_read_timeout 300s;` (minimal) untuk `location /api/v1/app/mikrotik/`. Tanpa itu
  klien bisa menerima 504 (gateway timeout) dari nginx padahal operasi di backend tetap selesai; klien lalu cenderung mengulang.
- Follow-up (belum dikerjakan): sesi binary per operasi (satu login untuk seluruh `apply`/`cleanup`) agar durasi turun.

## Kode error

`MIKROTIK_ROUTER_NOT_FOUND` (404), `MIKROTIK_ROUTER_NAME_TAKEN` (409), `VALIDATION_ERROR` (422),
`MIKROTIK_HOST_NOT_ALLOWED` (422), `MIKROTIK_TLS_REQUIRED` (422), `MIKROTIK_TLS_UNTRUSTED` (422), `MIKROTIK_SETUP_REST_UNVERIFIED` (422),
`MIKROTIK_AUTH_FAILED` (502), `MIKROTIK_UNREACHABLE` (502), `MIKROTIK_SETUP_PLAN_CHANGED` (409),
`MIKROTIK_SETUP_CONFLICT` (409), `MIKROTIK_SETUP_IN_PROGRESS` (409), `MIKROTIK_SETUP_FAILED` (502), `MIKROTIK_SETUP_STATE_NOT_SAVED` (500), `MIKROTIK_SETUP_MARK_FAILED` (500), `RATE_LIMITED` (429), `INTERNAL_ERROR` (500).

## Pemetaan PPP secret (Fase 2)

Semua endpoint di bawah `/api/v1/app/mikrotik`, tunduk pada entitlement `mikrotik.enabled`. Spesifikasi lengkap di `api/openapi.yaml`.

| Endpoint | Permission |
| --- | --- |
| `POST /routers/{id}/secrets/import` | `mikrotik_secret.import` |
| `GET /secrets`, `GET /accounts/search` | `mikrotik_secret.read` |
| `POST /secrets/{id}/pair`, `/pair-new-customer`, `PUT /secrets/{id}/original-profile` | `mikrotik_secret.pair` |
| `POST /secrets/{id}/unpair` | `mikrotik_secret.unpair` |
| `GET /customers/{account_id}/policy` | `mikrotik_policy.read` |
| `PUT /customers/{account_id}/policy` | `mikrotik_policy.update` |
| `GET /products/unmarked`, `GET /products/triggers`, `GET /products/{id}/trigger/preview` | `mikrotik_product_trigger.read` |
| `PUT /products/{id}/trigger` | `mikrotik_product_trigger.update` |

Permission dibuat oleh migration `000172`; tabel oleh `000171`. Hanya import dan `original-profile` menghubungi router,
sehingga hanya keduanya memakai rate limit probe (20 per menit per organization, bersama counter route router lain).

### Impor dan pairing

- Impor membaca `/ppp/secret` dan menyimpan salinan di cache lokal (tanpa password PPP). Hanya secret dengan `service`
  kosong, `any`, atau `pppoe` yang diimpor. Batas 10000 secret per impor (422 `VALIDATION_ERROR`, tidak ada yang ditulis).
- Aturan impor ulang: hanya kolom milik router (profile, disabled, comment, last_seen_at) yang diperbarui. Pairing,
  `original_profile`, dan `isolation_state` tidak pernah disentuh. `needs_original_profile` tidak pernah dihapus oleh
  impor; hanya `PUT .../original-profile` yang menghapusnya.
- `present=false`: secret yang tidak ada lagi di router ditandai, tidak dihapus. Secret `present=false` tidak bisa dipasangkan.
- `needs_original_profile=true`: secret ditemukan di profile isolir router (`isolir_profile_name`) dan belum punya
  `original_profile`. Pairing ditolak (409 `MIKROTIK_SECRET_NOT_PAIRABLE`) sampai `original_profile` ditetapkan; profile
  itu harus ada di `/ppp/profile` router dan bukan profile isolir (422 bila tidak).
- Satu secret hanya untuk satu pelanggan (409 `MIKROTIK_SECRET_ALREADY_PAIRED`); satu pelanggan boleh punya banyak secret.
  Saat dipasangkan, `original_profile` kosong diisi dari profile saat ini.
- Unpair mempertahankan `original_profile` dan ditolak (409 `MIKROTIK_SECRET_ISOLATED`) saat `isolation_state` bukan `normal`.
- `pair-new-customer` membuat kontak CRM (source `mikrotik_import`, tag `pppoe`, lifecycle customer) + akun pelanggan, lalu
  memasangkan. Pembuatan kontak mengikuti kuota CRM: kuota penuh memberi 409 `QUOTA_EXCEEDED`.

### Pengecualian dan penanda produk

- `PUT /customers/{account_id}/policy` menyimpan `exempt` + `reason` (wajib 1-500 karakter saat `exempt=true`). Pelanggan
  tanpa baris kebijakan dibaca sebagai `exempt=false`. Pada Fase 2 hanya menyimpan data; belum ada efek ke router.
- `PUT /products/{id}/trigger` menandai produk sebagai layanan PPPoE. `effective_from` adalah tanggal WIB (YYYY-MM-DD):
  menandai ulang produk yang sudah aktif mempertahankannya, mengaktifkan lagi setelah dinonaktifkan mengaturnya ke hari ini.
  `GET .../trigger/preview` menghitung dampak (kontrak aktif, pelanggan, invoice overdue) tanpa menulis.
  `GET /products/triggers` mendaftar semua penandaan (dengan nama/SKU produk, maks 200 diperkaya). Respons PUT tidak memuat dampak; ambil dari endpoint preview.
  `GET /products/unmarked` mendaftar produk recurring dalam kontrak aktif yang belum ditandai.

### Audit

Event ditulis setelah mutasi commit; kegagalan menulis audit hanya di-log dan tidak menggagalkan operasi:
`secret.imported`, `secret.paired`, `secret.unpaired`, `secret.original_profile_set`, `customer_policy.exempt_set`,
`product_trigger.set`.

### Kode error tambahan

`MIKROTIK_SECRET_NOT_FOUND` (404), `MIKROTIK_ACCOUNT_NOT_FOUND` (404), `MIKROTIK_PRODUCT_NOT_FOUND` (404),
`MIKROTIK_SECRET_ALREADY_PAIRED` (409), `MIKROTIK_SECRET_ISOLATED` (409), `MIKROTIK_SECRET_NOT_PAIRABLE` (409),
`QUOTA_EXCEEDED` (409, dari kuota CRM saat `pair-new-customer`).

### Batas yang diketahui

- Komentar secret tidak disalin ke kontak CRM (tabel kontak tidak punya kolom catatan); komentar tetap ada di cache secret.
- Pembuatan kontak+akun lalu pairing pada `pair-new-customer` tidak atomik: bila pairing kalah balapan, kontak/akun baru
  tertinggal tanpa pairing.
- Fase 2 tidak mengubah profile secret di router dan tidak mengisolir; itu Fase 3.

## Tidak termasuk Fase 1

Mesin isolir otomatis dari invoice overdue, halaman isolir publik,
dan wizard frontend. Lihat spec di atas (fase 2 sampai 4).

## Belum terbukti

Seluruh uji nyata hanya pada satu CHR RouterOS 7.8 lewat binary API (lab). Belum terbukti:

- RouterOS v6 (klien binary tersedia, tidak ada router v6 untuk uji).
- Transport REST secara fungsional (di lab `www-ssl` mati) dan binary API TLS (8729).
- Redirect HTTP (butuh `/ip proxy`; HTTPS tidak bisa dialihkan).
- Interaksi dengan fasttrack dan sesi PPPoE nyata (DNS ke router, pemberian alamat dari pool isolir, secret dengan `remote-address` sendiri).
- Akurasi allowlist FQDN saat IP CDN berganti.

Rincian per butir dan labelnya ada di runbook.
