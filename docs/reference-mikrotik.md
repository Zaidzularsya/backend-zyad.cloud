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

Registrasi (`POST /routers`) mewajibkan `port` >= 1 (maksimal 65535) di lapisan HTTP.

Respons router tidak pernah memuat password, hanya `has_password`. Tabel: `mikrotik_routers` (migration `000168`),
permission di migration `000169`.

## Entitlement dan tenant

Seluruh grup route butuh entitlement `mikrotik.enabled` (default mati; diaktifkan manual per organization) dan
organization aktif bertipe customer atau platform.

## Aturan keamanan

- **Guard SSRF**: host di-resolve dan hanya boleh publik, atau berada di CIDR privat yang diizinkan lewat env
  `MIKROTIK_ALLOWED_PRIVATE_CIDRS` (daftar dipisah koma; kosong berarti semua privat ditolak). Pelanggaran: `MIKROTIK_HOST_NOT_ALLOWED`.
- **TLS wajib untuk host publik** (`MIKROTIK_TLS_REQUIRED`). Sertifikat tidak tepercaya ditolak dengan
  `MIKROTIK_TLS_UNTRUSTED` yang membawa `fingerprint` dan `pin_mismatch` agar UI dapat menawarkan **pin fingerprint**
  (`tls_fingerprint`, SHA-256 leaf).
- **Password terenkripsi** di database dan tidak pernah dikembalikan; error tak dikenal tidak dibocorkan ke klien.
- **Redirect HTTP ditolak** oleh klien REST (tidak mengikuti redirect), untuk mencegah pengalihan ke host lain.
- **Batas respons**: 8 MiB per panggilan (REST dan binary), 1 MiB per word binary API.

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
  (ulangi operasi yang sama, idempoten).
- `cleanup` hanya menghapus objek `zyad-managed` dan aman diulang.
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
  klien bisa menerima 502 dari nginx padahal operasi di backend tetap selesai; klien lalu cenderung mengulang.
- Follow-up (belum dikerjakan): sesi binary per operasi (satu login untuk seluruh `apply`/`cleanup`) agar durasi turun.

## Kode error

`MIKROTIK_ROUTER_NOT_FOUND` (404), `MIKROTIK_ROUTER_NAME_TAKEN` (409), `VALIDATION_ERROR` (422),
`MIKROTIK_HOST_NOT_ALLOWED` (422), `MIKROTIK_TLS_REQUIRED` (422), `MIKROTIK_TLS_UNTRUSTED` (422),
`MIKROTIK_AUTH_FAILED` (502), `MIKROTIK_UNREACHABLE` (502), `MIKROTIK_SETUP_PLAN_CHANGED` (409),
`MIKROTIK_SETUP_CONFLICT` (409), `MIKROTIK_SETUP_FAILED` (502), `MIKROTIK_SETUP_STATE_NOT_SAVED` (500), `INTERNAL_ERROR` (500).

## Tidak termasuk Fase 1

Impor PPP secret, penanda produk untuk isolir, mesin isolir otomatis dari invoice overdue, halaman isolir publik,
dan wizard frontend. Lihat spec di atas (fase 2 sampai 4).

## Belum terbukti

Seluruh uji nyata hanya pada satu CHR RouterOS 7.8 lewat binary API (lab). Belum terbukti:

- RouterOS v6 (klien binary tersedia, tidak ada router v6 untuk uji).
- Transport REST secara fungsional (di lab `www-ssl` mati) dan binary API TLS (8729).
- Redirect HTTP (butuh `/ip proxy`; HTTPS tidak bisa dialihkan).
- Interaksi dengan fasttrack dan sesi PPPoE nyata (DNS ke router, pemberian alamat dari pool isolir, secret dengan `remote-address` sendiri).
- Akurasi allowlist FQDN saat IP CDN berganti.

Rincian per butir dan labelnya ada di runbook.
