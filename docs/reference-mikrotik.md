# Referensi Modul Mikrotik (Fase 1: fondasi router)

Status: **WIP**. Spec: `docs/superpowers/specs/2026-10-29-mikrotik-pppoe-isolir-design.md`.
Runbook penyiapan router (hasil uji lab): `docs/runbook-mikrotik-isolir-setup.md`.

## Tujuan

Tenant mendaftarkan router MikroTik miliknya, menguji koneksi, lalu menyiapkan objek isolir PPPoE di router
lewat alur preview, apply, cleanup. Fase 1 hanya fondasi; isolir otomatis ada di Fase 3 dan halaman publik di Fase 4.

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
- `present=false`: secret yang tidak ada lagi di router ditandai, tidak dihapus. Secret `present=false` tidak bisa dipasangkan. `POST /secrets/pair-bulk` yang dibatalkan di tengah jalan melaporkan sisa item sebagai `MIKROTIK_INTERNAL` tanpa diproses.
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
  tanpa baris kebijakan dibaca sebagai `exempt=false`. Setelah disimpan, akun dievaluasi ulang bila engine bukan `off` (hook `customer_policy_changed`); lihat "Pusat Kendali (Gelombang 2)".
  `exempted_until` (opsional, `YYYY-MM-DD`) membatasi pengecualian sampai akhir hari bisnis WIB itu (inklusif); hanya
  bermakna saat `exempt=true` (disimpan NULL bila `exempt=false`), harus >= hari ini (WIB) atau 422, null/kosong = tanpa
  batas. Audit `customer_policy.exempt_set` memuat `exempted_until`.
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
`MIKROTIK_SECRET_HAS_ACTIVE_TASK` (409, Fase 3), `MIKROTIK_ROUTER_HAS_ISOLATED_SECRETS` (409, Fase 3, hapus router),
`QUOTA_EXCEEDED` (409, dari kuota CRM saat `pair-new-customer`).

### Batas yang diketahui

- Komentar secret tidak disalin ke kontak CRM (tabel kontak tidak punya kolom catatan); komentar tetap ada di cache secret.
- Pembuatan kontak+akun lalu pairing pada `pair-new-customer` tidak atomik: bila pairing kalah balapan, kontak/akun baru
  tertinggal tanpa pairing.
- Fase 2 tidak mengubah profile secret di router dan tidak mengisolir; itu Fase 3.

## Mesin isolir (Fase 3)

Mesin ini mengisolir pelanggan PPPoE yang punya invoice lewat jatuh tempo dan membuka isolirnya setelah lunas. Seluruhnya
dikendalikan oleh `MIKROTIK_ISOLATION_MODE` (default `off`: tidak ada listener, tidak ada job, alur lama tidak berubah).

**Status uji:** unit test dan integration test (Postgres privat) lulus. Uji lab terhadap router nyata
(`TestLab_IsolateRestoreRoundTrip`, `internal/modules/mikrotik/service/isolation_lab_test.go`) dijalankan 1 Nov 2026:
**PASS untuk transport binary di RouterOS 7.8** (isolate, isolate idempoten, restore); REST di-skip (tidak tersedia di
lab) dan RouterOS v6 tidak diuji. Detail di `docs/runbook-mikrotik-isolir-setup.md` bagian 11.

### Aturan evaluasi

> **Diperbarui oleh Gelombang 2** (lihat "Pusat Kendali (Gelombang 2)"): kandidat memakai grace (`due_date < hari bisnis WIB - grace_days`,
> bukan sekadar "lebih awal dari hari ini"); state `isolated` juga berlaku selama uji pilot aktif; pengecualian bisa kedaluwarsa
> (`exempted_until`); dan aksi isolir digerbang oleh mode tenant (default `simulation`: tidak ada tugas isolir). Poin di bawah
> adalah aturan dasar Fase 3 dengan grace 0 dan mode tenant `active`.

- **Invoice kandidat** (`internal/app/mikrotik_overdue_source.go`): invoice akun dengan status `issued` atau `overdue`,
  `due_date` lebih awal dari hari ini (WIB), dan memuat item dengan produk yang penanda triggernya `enabled` serta
  `due_date >= effective_from`. Pembayaran sebagian tidak berpengaruh; invoice keluar dari himpunan hanya saat `paid`/`void`.
- **State yang diinginkan** per akun: `isolated` bila ada minimal satu invoice kandidat **dan** akun tidak dikecualikan
  (`exempt`); selain itu `normal`.
- **Aksi per secret yang dipasangkan ke akun**: `isolate` bila state diinginkan `isolated`, secret `normal`, `present=true`
  dan `needs_original_profile=false`; `restore` bila state diinginkan `normal` dan secret `isolated` (tanpa syarat `present`).
  Secret yang tidak dipasangkan tidak pernah diisolir; secret terisolir yang kehilangan pasangan tetap dipulihkan oleh processor.
- Evaluasi diulang di worker tepat sebelum menyentuh router; tugas yang tidak lagi sesuai dibatalkan (`superseded`).

### Outbox tugas, status, prioritas

Tabel `mikrotik_isolation_tasks`: satu tugas aktif (`pending`/`running`) per secret. Status: `pending`, `running`, `done`,
`failed`, `cancelled`. Prioritas: `restore` = 100, `isolate` = 10 (restore diambil lebih dulu). Menjadwalkan `isolate`
membatalkan `restore` pending (dan sebaliknya); tugas lawan yang sedang `running` dibiarkan dan akun dievaluasi ulang
setelah selesai. Lease `running` 2 menit (tugas worker yang mati diambil ulang setelah itu). Aksi router: ubah `profile`
secret, hapus sesi `/ppp/active` milik secret, baca ulang profile untuk konfirmasi; semua langkah idempoten. Isolasi tidak
pernah menimpa `original_profile` dengan profile isolir; restore menolak target kosong atau sama dengan profile isolir.

### Kunci per router

Sebelum menyentuh router, processor mengambil advisory lock `mikrotik:router:{router_id}`. Bila router sedang dipegang
worker lain, tugas dikembalikan ke `pending` 15 detik kemudian **tanpa** menambah `attempts`.

### Coba ulang dan backoff

Jeda setelah percobaan gagal ke-N adalah entri ke-N tabel (entri terakhir berulang). Interval job 2 menit, jadi jeda efektif
tidak pernah lebih pendek dari itu.

| Aksi | Jeda (setelah gagal ke-1, 2, 3, ...) | Maks percobaan | Notifikasi tenant mulai percobaan ke- |
|---|---|---|---|
| `isolate` | 1 mnt, 5 mnt, 15 mnt, 1 jam (berulang) | 8 | 5 |
| `restore` | 30 dtk, 1 mnt, 2 mnt, 5 mnt, 15 mnt (berulang) | 12 | 3 |

Kegagalan **permanen** (`router_not_found`, `router_not_ready`, `auth_failed`, `tls_untrusted`, `host_not_allowed`,
`secret_missing`, `profile_missing`, `restore_target_invalid`) langsung menjadi `failed` dan langsung memberi tahu tenant.
Kegagalan **sementara** (`unreachable`, `unknown`) dicoba ulang sampai anggaran habis lalu `failed` (tenant selalu diberi
tahu saat habis). Satu tugas memberi tahu tenant paling banyak sekali. `secret_missing` juga menandai secret `present=false`.
Setelah `failed`, tugas tidak dibuat ulang oleh processor; reconciler berikutnya mengevaluasi akun lagi dan membuat tugas
baru, atau operator memakai `POST /tasks/{id}/retry`.

**Masa jeda enqueue setelah gagal** (`domain.IsolateFailCooldown` 1 jam, `domain.RestoreFailCooldown` 15 menit):
`EnqueueIsolate`/`EnqueueRestore` tidak membuat tugas baru selama tugas `failed` terakhir untuk secret **dan aksi yang
sama** selesai kurang dari masa jeda itu (aksi lawan tidak terhalang). Tanpa ini reconciler membuat ulang tugas yang gagal
permanen setiap tick dan tenant menerima alert tiap kali. `POST /tasks/{id}/retry` tidak terkena masa jeda.

**Isolate parsial.** Bila `isolate` sudah memasang profile isolir di router lalu langkah berikutnya gagal (atau worker mati
di antara `Set` dan penandaan DB), DB masih `normal`. Sebelum membatalkan tugas `isolate` yang sudah pernah diambil
(`superseded` atau `entitlement_disabled`), processor membaca profile secret di router (read-only, di bawah kunci router);
bila profile isolir, DB diselaraskan ke `isolated` (audit `partial=true`) dan evaluasi lanjutan menjadwalkan `restore`.
Pembacaan yang gagal **tidak pernah** membatalkan tugas diam-diam: kegagalan sementara dihitung sebagai percobaan dan
dicoba ulang dengan backoff `isolate` (alert tenant mengikuti ambang biasa); kegagalan permanen (mis. `auth_failed`,
`host_not_allowed`, router terhapus) atau anggaran habis (8 percobaan) membuat tugas `failed` dengan kode
`router_unverifiable` ("Router tidak dapat diperiksa; periksa profile pelanggan secara manual."), audit `task.failed`, alert
tenant sekali, tanpa evaluasi lanjutan. Tugas `failed` tidak menghalangi hapus router/unpair dan bisa di-retry operator;
masa jeda enqueue mencegah tugas yang sama dibuat ulang terus. Karena `ClaimBatch` selalu mengisi `started_at`,
pembacaan ini terjadi untuk setiap pembatalan `isolate` yang diproses worker (satu `print`, tanpa tulis).

### Mode

| Mode | Efek |
|---|---|
| `off` (default; nilai tak dikenal dianggap `off`) | Tidak ada listener invoice/hook di API, tidak ada job di worker. |
| `dry_run` | Evaluasi berjalan (hook API, listener invoice, reconciler worker) tetapi hanya mencatat keputusan ke log (`mikrotik: dry-run decision`); **tidak ada tugas dibuat dan tidak ada router disentuh**. Worker tetap membangun dependensi notifikasi, sehingga **tenant storage harus terkonfigurasi di worker** atau worker gagal start. |
| `on` | Evaluasi membuat tugas **sesuai mode tenant** (default `simulation`: tidak ada tugas isolir; isolir hanya untuk `pilot`/`active`, lihat "Pusat Kendali"; restore tidak digerbang); worker memprosesnya ke router, menulis audit, dan mengirim notifikasi. Tugas yang tersisa dari periode `on` sebelumnya tidak disentuh selama mode `dry_run`/`off`. |

Mode dibaca dari env pada saat start (API dan worker): ubah lalu restart keduanya.

### Pemicu

- Listener receivable: `InvoicePaid`, `InvoicePaidReplay` (aman diulang), `InvoiceVoided` (evaluasi ulang akun pemilik invoice).
- Perubahan pengecualian pelanggan (`customer_policy_changed`) dan pair/unpair secret (`secret_pairing_changed`).
- **Reconcile periodik** tiap 2 menit di worker (`mikrotikIsolationInterval`; jendela kandidat memakai `grace_days` tenant, lihat "Pusat Kendali"): mengevaluasi akun yang punya invoice kandidat
  atau secret terisolir. Ini satu-satunya jalur untuk perubahan penanda produk (aktif/nonaktif, `effective_from`),
  jatuh tempo yang terlewati, dan pembuatan ulang tugas setelah `failed`. Hook langsung hanya mempercepat reaksi.
- Hook berjalan setelah perubahan committed, dengan timeout 5 detik, dan hanya mencatat log bila gagal.

### Hilangnya entitlement `mikrotik.enabled`

Organisasi tanpa entitlement (atau bila pengecekan entitlement error) diproses **restore-only**: reconciler hanya
mengevaluasi akun dengan secret terisolir dan tidak pernah membuat tugas `isolate`; tugas `isolate` yang terlanjur ada
dibatalkan (`entitlement_disabled`) tanpa menyentuh router; `restore` tetap berjalan, tidak bergantung `isolir_ready`.
Mode `dry_run` hanya mencatat log di jalur ini juga. Karena route `/app/mikrotik` dijaga entitlement yang sama, seluruh
grup UI/API (dashboard tugas, retry, pengecualian pelanggan) tidak dapat diakses selama entitlement mati, sementara
restore tetap berjalan otomatis.

Hanya **organisasi aktif** yang dikunjungi worker: pelanggan terisolir milik organisasi yang disuspend tidak dibuka
otomatis sampai organisasi aktif lagi.

### Penjaga hapus router dan unpair

- `DELETE /routers/{id}` ditolak 409 `MIKROTIK_ROUTER_HAS_ISOLATED_SECRETS` selama router punya secret `isolated` atau
  secret dengan tugas `pending`/`running` (dicek di UPDATE yang sama dengan soft delete).
- `POST /secrets/{id}/unpair` ditolak 409 `MIKROTIK_SECRET_HAS_ACTIVE_TASK` selama secret punya tugas `pending`/`running`
  (melengkapi aturan lama: secret terisolir tidak bisa di-unpair, 409 `MIKROTIK_SECRET_ISOLATED`).
- `PATCH /routers/{id}` yang mengubah host/kredensial **tidak** dijaga: mengarahkan router ke perangkat lain saat masih ada
  pelanggan terisolir membuat restore berjalan ke perangkat yang salah. Setup ulang/cleanup isolir mengosongkan
  `isolir_profile_name`, sehingga deteksi isolate parsial tidak bisa berjalan untuk router itu. Keduanya hanya
  didokumentasikan.

### Audit

Ditulis sebagai event sistem (tanpa aktor) oleh worker, kecuali `task.retried` yang memuat aktor: `isolation.isolated`
(metadata `partial=true` bila isolate gagal setelah profile terlanjur berubah dan DB diselaraskan), `isolation.restored`,
`task.failed`, `task.cancelled`, `task.retried`, `notification.failed` (notifikasi pelanggan gagal; tugas tidak diulang).

### Notifikasi

- Tenant: event `mikrotik.isolation_task_failed` (template email `id-ID`) dikirim ke setiap anggota yang memegang
  `mikrotik_task.read`, berisi nama router, nama secret, aksi, alasan tetap, dan tautan `/app/mikrotik/tasks`. Tidak
  memuat host, username, atau password router.
- Pelanggan: email/WhatsApp saat berhasil diisolir atau dibuka, memakai pengirim yang dikonfigurasi di pengaturan
  receivable tenant; sekali per tugas, kegagalan kirim tidak menggagalkan tugas.

### API dashboard tugas

`GET /api/v1/app/mikrotik/tasks` (filter `status`, `router_id`; `limit` default 50, maks 200; `offset`),
`GET /tasks/summary`, `POST /tasks/{id}/retry` (hanya `failed`; reset `attempts`). Detail di `api/openapi.yaml`.
Respons retry tidak mengisi `router.name` dan `account` (klien memuat ulang daftar). Kode error: `MIKROTIK_TASK_NOT_FOUND`
(404), `MIKROTIK_TASK_NOT_RETRYABLE` (409), `MIKROTIK_TASK_ACTIVE_EXISTS` (409), `VALIDATION_ERROR` (400, filter tidak valid).

Permission `mikrotik_task.read` dan `mikrotik_task.retry` dibuat oleh migration 000174 dan diberikan ke
`organization_owner`. Migration 000175 menambah `mikrotik_task.read` ke peran `member`: anggota biasa boleh membuka
dashboard tugas (`GET /tasks`, `GET /tasks/summary`) tetapi `POST /tasks/{id}/retry` tetap 403, begitu pula seluruh
route Mikrotik lain (router, secret, kebijakan, onboarding). Notifikasi kegagalan tugas tetap hanya ke pemilik.

## UI tenant gelombang 1: analisis, pairing massal, onboarding

Semua di bawah `/api/v1/app/mikrotik`, tunduk pada entitlement `mikrotik.enabled`. Migration `000175` membuat tabel saran
(`mikrotik_secret_suggestions`) dan grant member di atas. Skema lengkap di `api/openapi.yaml`.

| Endpoint | Permission | Catatan |
| --- | --- | --- |
| `POST /routers/{id}/secrets/analyze` | `mikrotik_secret.import` | Hitung ulang saran pairing satu router; hasil `{total, suggested, needs_decision}`. Tidak login ke router, tetapi memakai rate limit probe. |
| `GET /secrets?suggestion=suggested\|needs_decision` (juga `needs_original_profile=true\|false`) | `mikrotik_secret.read` | Filter `suggestion` hanya berlaku untuk secret present yang belum dipasangkan. Tiap secret memuat `suggestion` (`null` bila tidak ada; juga `null` untuk secret yang sudah dipasangkan atau `present=false`). Secret milik router yang sudah dihapus (soft delete) tidak ditampilkan, dan juga tidak dihitung di onboarding maupun status pelanggan. Pair/unpair menghapus saran lama secret itu. Catatan: `account_name` (nama pelanggan terpasang; `""` bila belum dipasangkan atau akun dihapus) hanya diisi oleh endpoint daftar ini. |
| `POST /secrets/pair-bulk` | `mikrotik_secret.pair` | 1-200 item `{secret_id, account_id}`, tidak atomik, rate limit probe. |
| `GET /onboarding` | `mikrotik_router.read` | Checklist kesiapan isolir. |
| `GET /customers/status?account_ids=a,b` | `mikrotik_policy.read` | Status isolir per akun, maks 100 id unik. |

- Pencocokan saran: nomor telepon lebih dulu, lalu nama persis (tanpa beda huruf besar/kecil, di-trim). Lebih dari satu akun
  pada level pemenang menjadi `basis=ambiguous` (field `account_*` kosong, `candidates` >= 2) dan masuk `needs_decision`.
  `suggested` = satu kecocokan phone/name. Secret yang belum dianalisis juga masuk `needs_decision`.
- `pair-bulk`: kegagalan per item dilaporkan di `results[]` (HTTP 200) dengan `code` per baris
  (`MIKROTIK_SECRET_NOT_FOUND`, `MIKROTIK_ACCOUNT_NOT_FOUND`, `MIKROTIK_ROUTER_NOT_FOUND`, `MIKROTIK_SECRET_ALREADY_PAIRED`,
  `MIKROTIK_SECRET_NOT_PAIRABLE`, `MIKROTIK_INTERNAL`). Hanya body salah bentuk (items kosong/>200, uuid tidak valid,
  `secret_id` berulang, field tidak dikenal) yang gagal seluruhnya dengan 422 `VALIDATION_ERROR`.
- `analyze` pada id router yang bukan uuid menjawab 404 `MIKROTIK_ROUTER_NOT_FOUND`. Analisis kedua untuk router yang sama
  saat yang pertama berjalan: 409 `MIKROTIK_ANALYZE_IN_PROGRESS` (tidak ada yang berubah).
- `onboarding.complete` = router siap (`isolir_ready`) dan >= 1 secret terpasang dan tidak ada secret yang menunggu
  `original_profile` dan >= 1 produk ditandai dan pengirim default receivable terisi. Secret dihitung hanya yang present.
- `customers/status`: `isolation_state` = `isolated` (ada secret terpasang yang terisolir), `normal` (ada secret terpasang,
  tidak ada yang terisolir), `none` (tanpa secret terpasang, walau `exempt`). `account_ids` kosong/bukan uuid/lebih dari 100: 422.
  Setiap item juga memuat `exempted_until` (nilai tersimpan, `YYYY-MM-DD` atau null; pengecualian yang sudah lewat tetap
  dilaporkan dengan `exempt` sesuai data, klien membandingkan tanggal) dan `pending_isolation` (diprediksi akan diisolir bila isolir aktif, tidak bergantung pada mode tenant atau env; status preview `will_isolate`, lihat "Pusat Kendali").
- Peran `member` hanya memegang `mikrotik_task.read`, jadi di UI tenant ia hanya melihat dashboard tugas (read-only).
- Kode error baru: `MIKROTIK_ANALYZE_IN_PROGRESS` (409).

### Batas yang diketahui

- RouterOS v6 dan transport REST **belum terbukti** untuk isolir/buka-isolir; uji lab Fase 3 hanya lulus untuk binary di
  RouterOS 7.8 dengan secret uji disabled (lihat atas).
- Sesi lama benar-benar terputus dan sesi baru mendapat profile isolir: belum terbukti dengan CPE nyata.
- Redirect HTTP bersifat best-effort (HTTPS tidak bisa dialihkan).
- Pembuatan ulang secret dengan nama sama di router (setelah dihapus) tidak diwarisi pairing-nya; tetap catatan terbuka.
- Organisasi tanpa entitlement tetap dikunjungi job tiap 2 menit bila punya secret terisolir (restore-only); secara umum
  job menelusuri semua organisasi aktif sehingga ada noise antrean/log untuk organisasi yang tidak berhak.
- Satu tenant yang lambat (router tak terjangkau, timeout) dapat menunda tenant lain dalam satu tick worker; anggaran
  waktu per tick per tenant belum diimplementasikan.

## Pusat Kendali (Gelombang 2)

Pusat kendali memberi tenant kemudi atas mesin isolir (Fase 3): mode peluncuran bertahap, toleransi keterlambatan,
akun pilot, pratinjau kandidat, pemulihan massal, dan log aktivitas. Semua di bawah `/api/v1/app/mikrotik/control`,
tunduk pada entitlement `mikrotik.enabled`. Skema lengkap dan kode error ada di `api/openapi.yaml`. Tabel:
`mikrotik_org_settings`, `mikrotik_pilot_accounts`, kolom `mikrotik_customer_policies.exempted_until` (migration `000176`),
permission (`000177`), index audit (`000178`).

### Endpoint dan permission

| Endpoint | Permission | Catatan |
| --- | --- | --- |
| `GET /control` | `mikrotik_control.read` | Mode, grace, `engine`, `effective_mode`, hitungan pilot, `isolated_secrets` |
| `PUT /control/grace-days` | `mikrotik_control.update` | `{grace_days}` 0..60 |
| `PUT /control/mode` | `mikrotik_control.update` | `{mode, exemptions?}` |
| `GET /control/pilot` | `mikrotik_control.read` | Daftar akun pilot |
| `PUT /control/pilot/{account_id}`, `DELETE /control/pilot/{account_id}` | `mikrotik_control.update` | 204, idempoten |
| `POST /control/pilot/{account_id}/test` (202), `DELETE .../test` (204) | `mikrotik_control.update` | Mulai/hentikan uji pilot |
| `POST /control/restore-all` | `mikrotik_control.restore` | Hanya saat env `on` dan mode `paused` |
| `GET /control/preview`, `/preview/summary`, `/preview/export` | `mikrotik_control.read` | Pratinjau kandidat, ringkasan, CSV |
| `GET /control/activity` | `mikrotik_control.read` | Log aktivitas |

`mikrotik_control.read`: `organization_owner` dan `member`. `mikrotik_control.update` dan `mikrotik_control.restore`: hanya
`organization_owner` (member mendapat 403 pada setiap mutasi). Tidak satu pun rute ini menghubungi router secara langsung
(tugas yang melakukannya), jadi rute ini tidak memakai rate limit probe.

Kode error khusus: 409 `MIKROTIK_PILOT_REQUIRED` (aktivasi sebelum pilot terverifikasi), `MIKROTIK_NOT_PAUSED` (restore-all
di luar `paused`), `MIKROTIK_ENGINE_OFF` (uji pilot atau restore-all saat env bukan `on`), `MIKROTIK_PILOT_MODE_REQUIRED` (uji pilot saat mode
bukan `pilot`), `MIKROTIK_PILOT_TEST_NOT_RUNNABLE` (akun dikecualikan, tanpa secret yang dapat diisolir, atau diblokir mode);
422 `MIKROTIK_MODE_INVALID` (mode tidak dikenal, atau `exemptions` bersama mode selain `active`) dan `VALIDATION_ERROR`;
404 `MIKROTIK_ACCOUNT_NOT_FOUND`.

### Mode dan tabel aturan

Dua lapis: env `MIKROTIK_ISOLATION_MODE` (saklar operator) dan mode tenant (`simulation`, `pilot`, `active`, `paused`).
**Env selalu menang.** Mode tenant tidak pernah memblokir pemulihan (restore); yang digerbang hanya aksi isolir. Tetapi tugas hanya dibuat dan diproses bila env `on`: pada env `off`/`dry_run` tidak ada restore otomatis maupun `restore-all`.

| Env \ tenant | `simulation` | `pilot` | `active` | `paused` |
| --- | --- | --- | --- | --- |
| `off` | tidak ada tugas dibuat atau dijalankan (termasuk restore) | sama | sama | sama |
| `dry_run` | hanya log keputusan, tanpa tugas (termasuk restore) | sama | sama | sama |
| `on` | isolir diblokir (alasan `simulation`), keputusan dicatat di log; restore berjalan | isolir hanya untuk akun pilot, selain itu `not_in_pilot`; restore berjalan | isolir untuk semua kandidat; restore berjalan | isolir diblokir (`paused`); restore berjalan |

- **Default `simulation`** bila organisasi belum punya baris di `mikrotik_org_settings`. Akibatnya dengan env `on`, tenant
  baru **tidak mengisolir siapa pun** sampai ia memilih `pilot` atau `active`.
- `active` hanya dapat dipilih setelah `pilot_verified_at` terisi (dari mode asal apa pun), kalau tidak 409
  `MIKROTIK_PILOT_REQUIRED`. `exemptions` hanya diterima bersama `active` (maks 500 akun, `reason` wajib, `until` opsional)
  dan disimpan satu transaksi dengan mode.
- `effective_mode` pada `GET /control`: `off`/`dry_run` bila env demikian, selain itu mode tenant.
- Tugas isolir yang sudah di antrean tetapi gerbang menutup sebelum dijalankan dibatalkan dengan alasan `mode_blocked`.
  Bila saat itu router sudah terlanjur memakai profile isolir (isolir parsial), secret ditandai terisolir lalu dijadwalkan
  restore dengan event `mode_blocked_restore` yang tetap berjalan walau akun masih menunggak. Pelanggan **tidak** menerima
  notifikasi "dipulihkan" untuk restore itu (ia tidak pernah diberi tahu terisolir).
- Bila pengaturan kendali tenant tidak dapat dibaca (galat DB), isolir diblokir dengan alasan `control_unreadable`
  (gagal-tertutup) dan pemulihan tetap berjalan. Konsekuensi: pembacaan gagal memakai grace 0 untuk evaluasi itu, sehingga
  restore bisa tertunda untuk pelanggan yang invoice terbarunya sebenarnya masih di dalam jendela grace asli.

### Grace days dan pengecualian

- Rumus: invoice menjadi kandidat bila `due_date < hari bisnis WIB - grace_days` (cutoff = hari bisnis dikurangi grace;
  `due_date` tepat di cutoff belum kandidat). Rentang 0..60; 0 = perilaku Gelombang 1.
- Menaikkan `grace_days` dapat memulihkan pelanggan terisolir yang invoice terlambatnya jatuh dalam jendela baru; grace
  tidak pernah mencegah pemulihan. Menurunkannya memperbanyak kandidat.
- `exempted_until` (di `PUT /customers/{account_id}/policy`, `customers/status`, `preview`, dan `exemptions.until`) adalah
  hari bisnis WIB terakhir pengecualian, **inklusif**; kosong/null = tanpa batas. Tidak boleh sebelum hari ini. Setelah lewat,
  pengecualian tidak lagi berlaku, dan akun menjadi kandidat pada reconcile berikutnya (nilainya tetap dilaporkan apa
  adanya; klien membandingkan tanggal).
- `customers/status.pending_isolation` = akun itu diprediksi akan diisolir bila isolir aktif (status preview `will_isolate`; tidak bergantung pada mode tenant atau env).

### Uji pilot

1. Syarat: env `on`, mode tenant `pilot`, akun ada di daftar pilot dan punya secret terpasang. Selain itu 409/422.
2. `POST .../test` membuka jendela **10 menit**, mengevaluasi akun, dan menjadwalkan isolir (walau akun tidak punya
   invoice kandidat). Bila tidak ada yang bisa diisolir, jendela diakhiri dan hasilnya 409
   `MIKROTIK_PILOT_TEST_NOT_RUNNABLE` (audit `pilot.test_failed`).
3. `DELETE .../test` mengevaluasi ulang akun saat itu juga sehingga restore dijadwalkan segera. Bila jendela habis begitu saja, restore menunggu reconcile berikutnya (sekitar 2 menit).
4. `pilot_verified_at` (tingkat organisasi, sekali) terisi hanya setelah ada tugas isolir `done` sejak uji dimulai **lalu**
   restore selesai. `verified` per akun di `GET /control/pilot` adalah heuristik (verifikasi organisasi terjadi sesudah uji
   terakhir akun itu dimulai).
5. Akun pilot **tidak pernah menerima notifikasi pelanggan** (isolir maupun pemulihan). Menghapus akun dari pilot
   menghentikan uji dan mengevaluasi ulang akun; isolir karena uji dipulihkan, tetapi akun yang masih punya invoice kandidat
   tidak dipulihkan (ia tetap kandidat sesuai mode tenant).

### Pratinjau kandidat

`status` memprediksi apa yang akan dilakukan engine terhadap akun yang punya invoice kandidat. Urutan prioritas (yang
pertama cocok menang): `exempt` → `no_secret` → `already_isolated` (semua secret sudah terisolir) → `will_isolate` (ada secret
yang dapat diisolir: normal, present, original profile diketahui, router `isolir_ready` dan profile isolir terisi) →
`needs_profile` → `secret_missing` → `router_not_ready`. `will_isolate` adalah **prediksi aksi isolir yang bisa berjalan**
dan ditampilkan apa pun mode tenant, artinya "bila aktif".

`GET /preview/summary`: `target` (will_isolate), `exempt`, `blocked` (no_secret + needs_profile + secret_missing +
router_not_ready), `already_isolated`, `paired_accounts`, `safe` = `paired_accounts - target` (minimal 0), `grace_days`,
`cutoff` (YYYY-MM-DD). Daftar dipaginasi (`page`, `per_page` default 50 maks 100, `meta`), filter `status` dan `q`.

**Ekspor CSV** (`GET /preview/export`, filter sama): maksimal 100.000 baris, dibaca keyset 1000 baris per halaman dan
dialirkan; header `Content-Disposition: attachment`, `X-Total-Count`, `Cache-Control: no-store`. Sel yang dapat terbaca
sebagai rumus diberi awalan `'`: diawali tab/CR, atau karakter pertama setelah spasi Unicode di depan (dipangkas dulu) adalah `= + - @`. Berkas memuat **nama dan telepon pelanggan** dan hanya butuh
permission baca. Galat sebelum byte pertama menjawab JSON; galat sesudah respons dimulai memutus koneksi sehingga unduhan
tampak tidak lengkap (bukan berkas utuh palsu).

### Restore-all

`POST /control/restore-all` butuh env `MIKROTIK_ISOLATION_MODE=on` (selain itu 409 `MIKROTIK_ENGINE_OFF`, diperiksa lebih dulu karena tidak ada worker yang memproses tugasnya) dan mode tenant `paused` (selain itu 409 `MIKROTIK_NOT_PAUSED`). Pada env `off`/`dry_run`, pulihkan pelanggan secara manual (runbook bagian 14.6). Ia **mengantrekan** tugas
restore untuk tiap secret terisolir (halaman 200; mode dibaca ulang tiap halaman) lalu segera menjawab; router baru
disentuh oleh processor. Hasil `{enqueued, skipped, failed, aborted}`: `skipped` = tidak memenuhi syarat, sudah punya
tugas aktif, atau dalam masa jeda; `failed` = enqueue gagal (dicatat di log, tidak menghentikan); `aborted=true` bila
tenant meninggalkan `paused` di tengah proses (hitungan parsial). Aman diulang. Tidak pernah melepas pairing.

### Log aktivitas dan audit

`GET /control/activity` (limit default 50, maks 100, offset; `meta.total`) menampilkan event audit dari daftar putih:
`isolation.isolated`, `isolation.restored`, `task.failed`, `task.cancelled`, `task.retried`, `control.mode_changed`,
`control.grace_days_changed`, `control.restore_all`, `pilot.added`, `pilot.removed`, `pilot.test_started`,
`pilot.test_stopped`, `pilot.test_failed`, `customer_policy.exempt_set`. Event pengelolaan router/secret sengaja tidak
ditampilkan. `message` berbahasa Indonesia dan hanya memuat nilai berbentuk kode dari metadata. Event audit yang ditulis
pusat kendali: `control.mode_changed`, `control.grace_days_changed`, `control.restore_all`, `pilot.added`, `pilot.removed`,
`pilot.test_started`, `pilot.test_stopped`, `pilot.test_failed`, `customer_policy.exempt_set`.

## Halaman Isolir Publik

Fase 4 menyediakan halaman publik tempat pelanggan terisolir melihat tagihan dan membayar. Backend:
`GET /api/v1/public/isolir/info` dan `POST /api/v1/public/isolir/lookup`, keduanya **tanpa autentikasi** dan terdaftar
hanya pada grup publik yang me-resolve tenant dari host (bukan grup terautentikasi). Header pemilih organisasi ditolak
400 `PUBLIC_ORGANIZATION_SELECTOR_FORBIDDEN`; host tak terdaftar 404 `PUBLIC_HOST_NOT_FOUND`.

- `info` -> `{"enabled": bool, "tenant": {"name","phone","email"}}`; `tenant` hilang bila `enabled=false`
  (entitlement `mikrotik.enabled` mati).
- `lookup` body `{"query": "<username PPPoE atau telepon>"}` -> `{"found","masked_name","isolated","invoices":[{"number","due_date","balance","pay_url"}]}`.
  `pay_url` kosong bila tenant belum punya pengirim default.

Aturan keamanan:

- **Jawaban seragam**: tidak ada kecocokan, tanpa tunggakan, kecocokan ganda, fitur mati, query kosong, lebih dari 100
  karakter, karakter kontrol, body rusak/tanpa `query`/lebih dari 2 KiB: semuanya HTTP 200 dengan body identik
  `{"found":false,"masked_name":"","isolated":false,"invoices":[]}`. Endpoint tidak boleh menjadi oracle enumerasi.
- Respons tidak memuat telepon/email pelanggan atau nama router/secret/profile/host; nama pelanggan disamarkan.
- Rate limit (Redis, fail open): per IP 20/menit, per query ternormalisasi 5/menit (kunci memakai hash, bukan nilai
  mentah), per tenant 300/menit. Melebihi batas: 429 `RATE_LIMITED` dengan body yang sama untuk akun ada maupun tidak.
- Galat service/port: satu body 500 `INTERNAL_ERROR` untuk semua penyebab; log server tidak memuat query.
- Setiap respons membawa `Cache-Control: no-store` dan `X-Robots-Tag: noindex`.
- `today` untuk penentuan tunggakan adalah hari bisnis WIB (`businesstime`).

Kode error: `PUBLIC_ORGANIZATION_SELECTOR_FORBIDDEN` (400), `PUBLIC_HOST_NOT_FOUND` (404), `RATE_LIMITED` (429),
`INTERNAL_ERROR` (500).

## Tidak termasuk Fase 1

Wizard/UI frontend halaman isolir (mesin isolir ada di Fase 3 di atas). Lihat spec di atas (fase 2 sampai 4).

## Belum terbukti

Seluruh uji nyata hanya pada satu CHR RouterOS 7.8 lewat binary API (lab). Belum terbukti:

- RouterOS v6 (klien binary tersedia, tidak ada router v6 untuk uji).
- Transport REST secara fungsional (di lab `www-ssl` mati) dan binary API TLS (8729).
- Redirect HTTP (butuh `/ip proxy`; HTTPS tidak bisa dialihkan).
- Interaksi dengan fasttrack dan sesi PPPoE nyata (DNS ke router, pemberian alamat dari pool isolir, secret dengan `remote-address` sendiri).
- Akurasi allowlist FQDN saat IP CDN berganti.

Rincian per butir dan labelnya ada di runbook.
