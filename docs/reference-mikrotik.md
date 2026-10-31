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
(`TestLab_IsolateRestoreRoundTrip`, `internal/modules/mikrotik/service/isolation_lab_test.go`) **belum pernah dijalankan**
karena kredensial lab tidak tersedia saat Task 10 dikerjakan; hasilnya tidak boleh dianggap lulus.

### Aturan evaluasi

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
Bila pembacaan gagal sementara, tugas dikembalikan ke `pending` dan tidak dibatalkan. Karena `ClaimBatch` selalu mengisi
`started_at`, pembacaan ini terjadi untuk setiap pembatalan `isolate` yang diproses worker (satu `print`, tanpa tulis).

### Mode

| Mode | Efek |
|---|---|
| `off` (default; nilai tak dikenal dianggap `off`) | Tidak ada listener invoice/hook di API, tidak ada job di worker. |
| `dry_run` | Evaluasi berjalan (hook API, listener invoice, reconciler worker) tetapi hanya mencatat keputusan ke log (`mikrotik: dry-run decision`); **tidak ada tugas dibuat dan tidak ada router disentuh**. Worker tetap membangun dependensi notifikasi, sehingga **tenant storage harus terkonfigurasi di worker** atau worker gagal start. |
| `on` | Evaluasi membuat tugas; worker memprosesnya ke router, menulis audit, dan mengirim notifikasi. Tugas yang tersisa dari periode `on` sebelumnya tidak disentuh selama mode `dry_run`/`off`. |

Mode dibaca dari env pada saat start (API dan worker): ubah lalu restart keduanya.

### Pemicu

- Listener receivable: `InvoicePaid`, `InvoicePaidReplay` (aman diulang), `InvoiceVoided` (evaluasi ulang akun pemilik invoice).
- Perubahan pengecualian pelanggan (`customer_policy_changed`) dan pair/unpair secret (`secret_pairing_changed`).
- **Reconcile periodik** tiap 2 menit di worker (`mikrotikIsolationInterval`): mengevaluasi akun yang punya invoice kandidat
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

Permission `mikrotik_task.read` dan `mikrotik_task.retry` (migration 000174) saat ini hanya diberikan ke
`organization_owner`. **Item terbuka:** apakah peran anggota biasa boleh membaca dashboard; hanya pemilik yang menerima
notifikasi kegagalan sampai itu diputuskan.

### Batas yang diketahui

- RouterOS v6 dan transport REST **belum terbukti** untuk isolir/buka-isolir; uji lab Fase 3 belum dijalankan (lihat atas).
- Sesi lama benar-benar terputus dan sesi baru mendapat profile isolir: belum terbukti dengan CPE nyata.
- Redirect HTTP bersifat best-effort (HTTPS tidak bisa dialihkan).
- Pembuatan ulang secret dengan nama sama di router (setelah dihapus) tidak diwarisi pairing-nya; tetap catatan terbuka.
- Organisasi tanpa entitlement tetap dikunjungi job tiap 2 menit bila punya secret terisolir (restore-only); secara umum
  job menelusuri semua organisasi aktif sehingga ada noise antrean/log untuk organisasi yang tidak berhak.
- Satu tenant yang lambat (router tak terjangkau, timeout) dapat menunda tenant lain dalam satu tick worker; anggaran
  waktu per tick per tenant belum diimplementasikan.

## Tidak termasuk Fase 1

Halaman isolir publik dan wizard/UI frontend (mesin isolir ada di Fase 3 di atas). Lihat spec di atas (fase 2 sampai 4).

## Belum terbukti

Seluruh uji nyata hanya pada satu CHR RouterOS 7.8 lewat binary API (lab). Belum terbukti:

- RouterOS v6 (klien binary tersedia, tidak ada router v6 untuk uji).
- Transport REST secara fungsional (di lab `www-ssl` mati) dan binary API TLS (8729).
- Redirect HTTP (butuh `/ip proxy`; HTTPS tidak bisa dialihkan).
- Interaksi dengan fasttrack dan sesi PPPoE nyata (DNS ke router, pemberian alamat dari pool isolir, secret dengan `remote-address` sendiri).
- Akurasi allowlist FQDN saat IP CDN berganti.

Rincian per butir dan labelnya ada di runbook.
