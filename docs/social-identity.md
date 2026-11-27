# Social Identity (Tahap 1)

Dokumen ini menjelaskan modul `social` Tahap 1: identitas warga (akun tanpa workspace), klaim relasi
pelanggan ke kontak CRM tenant, dan atribusi referral. Kontrak endpoint yang berlaku ada di
`api/openapi.yaml` (tag `Social`, `Auth`); dokumen ini hanya merangkum alur dan aturannya.

## Tujuan

- User bisa mendaftar sebagai warga (`social`) tanpa dibuatkan workspace/organisasi.
- User yang email-nya terverifikasi bisa menautkan dirinya ke kontak CRM sebuah tenant (relasi pelanggan).
- Setiap user punya kode referral; pendaftar yang datang lewat tautan dikreditkan ke pemilik kode.

## Registrasi dan `intent`

- `POST /api/v1/auth/register` (email + password) membuat akun aktif dengan email belum terverifikasi,
  mengantrekan email verifikasi, dan langsung login. Dimatikan secara default (`AUTH_PASSWORD_REGISTER_ENABLED`).
- `POST /api/v1/auth/google` dan `POST /api/v1/auth/google/callback` menerima `intent` yang sama.
- `intent`: `social` | `operator`.
  - `operator`: membuat workspace pribadi (perilaku lama dashboard web).
  - `social`: tanpa workspace; `GET /organizations` kosong sampai user dibuat/diundang ke organisasi.
  - Bila kosong, default = `operator` pada endpoint web yang sudah ada agar tidak breaking. **Aplikasi mobile
    wajib mengirim `intent=social`.** Nilai lain menghasilkan 422.
- `attribution` (opsional) ikut dikirim saat registrasi; diproses best-effort dan tidak pernah menggagalkan
  registrasi.

## Akun belum terverifikasi

- Akun dengan `email_verified_at IS NULL` boleh login dan membaca.
- Semua endpoint tulis sosial (Tahap 1: seluruh endpoint klaim milik user) menolak dengan
  `403 EMAIL_NOT_VERIFIED`. Pengecualian: `POST /social/me/customer-invites/accept`, karena token undangan
  membuktikan kepemilikan alamat email yang diundang.
- Login Google atas email yang sama dengan akun password **belum terverifikasi** tidak menautkan, tetapi
  mengambil alih akun (takeover): password dihapus, email ditandai terverifikasi, akun `pending` menjadi
  `active`, semua sesi dicabut, dan kejadian diaudit. Alasannya: pemilik email sebenarnya (terbukti lewat
  Google `email_verified`) tidak boleh terkunci oleh pendaftar yang tidak pernah membuktikan email itu.

## Klaim relasi pelanggan

Relasi disimpan di `customer_links` (status `pending_user`, `active`, `revoked`). Tabel ini global (tanpa RLS),
sehingga query sisi tenant selalu memfilter `organization_id` secara eksplisit. Relasi pelanggan tidak pernah
dikembalikan oleh endpoint publik.

**Kontak yang boleh diklaim (eligible):** tidak terhapus (`deleted_at IS NULL`) dan punya
`receivable_accounts` (`source_type='crm_contact'`, belum terhapus) **atau** onboarding PPPoE
(`mikrotik_secret_onboardings`) dengan `stage` `confirmed`/`paired`. Satu kontak hanya punya satu relasi hidup.

Tiga pemicu:

| Pemicu | Alur | Bukti kepemilikan |
|---|---|---|
| A. Email match | `GET /social/me/customer-claims` menawarkan kontak yang `lower(btrim(email))`-nya sama dengan email terverifikasi user; user `accept` atau `decline` | Email user terverifikasi |
| B. Undangan staff | Staff `POST /app/social/customer-invites` mengirim email berisi `<SOCIAL_CLAIM_URL_BASE>/claim?token=...`; user `POST /social/me/customer-invites/accept` | Token undangan (sekali pakai, kedaluwarsa 7 hari, disimpan sebagai hash; kirim ulang mencabut token lama) |
| C. Permintaan staff | Staff `POST /app/social/customer-links` untuk user terdaftar (cari via email) menghasilkan `pending_user`; muncul sebagai `kind=staff_request` di daftar klaim user dan baru `active` setelah user `accept` | Persetujuan user |

Aturan penawaran email match (A): tidak ditawarkan bila tenant mematikan `customer_claim_auto_offer`,
kontak sudah punya relasi hidup, user sudah punya relasi hidup di organisasi itu, atau user pernah
`decline` kontak tersebut. Permintaan staff (C) tetap ditawarkan walau opt-out aktif; opt-out juga tidak
menghapus relasi yang sudah ada.

**Opt-out tenant:** `GET/PUT /app/social/settings` (`customer_claim_auto_offer`, default `true`).

Kontak CRM tidak pernah ditautkan ke user hanya karena email sama tanpa bukti kepemilikan email.
User dapat mencabut relasinya sendiri (`DELETE /social/me/customer-links/{id}`); staff dapat mencabut dari
sisi tenant (`DELETE /app/social/customer-links/{id}`). Semua perubahan relasi masuk audit log
(`social.customer_link_*`).

## Atribusi referral

- Kode referral: 8 karakter dari alfabet `23456789ABCDEFGHJKLMNPQRSTUVWXYZ`, unik, ditetapkan saat pertama
  diminta (`GET /social/me/attribution`) dan tidak pernah berubah.
- `share_url` = `<SOCIAL_SHARE_URL_BASE>/r/<kode>`.
- First-touch, berlaku 30 hari: kode yang pertama kali dilihat lebih dari 30 hari sebelum registrasi diabaikan.
- `users.referred_by_user_id` hanya diisi sekali dan tidak pernah ditimpa.
- Kode milik sendiri ditolak; kode tidak dikenal/malformed diabaikan tanpa error (tidak membocorkan kode
  yang ada).
- Peristiwa tersimpan di `attribution_events` (`visit`, `signup`, `became_customer`). `became_customer`
  dikreditkan sekali, pada klaim relasi pertama user (best-effort). IP kunjungan hanya disimpan sebagai
  hash SHA-256.

## Environment variable baru

| Variabel | Default | Fungsi |
|---|---|---|
| `AUTH_PASSWORD_REGISTER_ENABLED` | `false` | Mengaktifkan `POST /auth/register`; bila mati endpoint menjawab 403 `AUTH_REGISTRATION_DISABLED` |
| `SOCIAL_SHARE_URL_BASE` | URL frontend | Basis `share_url` (`<base>/r/<kode>`) |
| `SOCIAL_CLAIM_URL_BASE` | URL frontend | Basis tautan undangan klaim (`<base>/claim?token=...`) |

## Endpoint

Auth (lihat `api/openapi.yaml`): `POST /api/v1/auth/register`, `POST /api/v1/auth/google`,
`POST /api/v1/auth/google/callback`.

Sisi user (tanpa konteks tenant):

| Method | Path | Fungsi |
|---|---|---|
| GET | `/api/v1/social/me/attribution` | Kode referral, `share_url`, total visit/signup |
| GET | `/api/v1/social/me/customer-relations` | Relasi pelanggan milik user |
| GET | `/api/v1/social/me/customer-claims` | Tawaran klaim (email match dan permintaan staff) |
| POST | `/api/v1/social/me/customer-claims/accept` | Terima tawaran |
| POST | `/api/v1/social/me/customer-claims/decline` | Tolak tawaran |
| POST | `/api/v1/social/me/customer-invites/accept` | Tebus token undangan staff |
| DELETE | `/api/v1/social/me/customer-links/{id}` | Cabut relasi milik sendiri |

Sisi tenant (staff):

| Method | Path | Permission |
|---|---|---|
| POST | `/api/v1/app/social/customer-invites` | `contact.update` |
| POST | `/api/v1/app/social/customer-links` | `contact.update` |
| GET | `/api/v1/app/social/customer-links` | `contact.update` |
| DELETE | `/api/v1/app/social/customer-links/{id}` | `contact.update` |
| GET | `/api/v1/app/social/settings` | `lead.read` |
| PUT | `/api/v1/app/social/settings` | `crm_settings.update` |

## Migration

- `000189_social_identity`: tabel `customer_links`, `customer_claim_invites`, `social_tenant_settings`,
  `attribution_events`, kolom referral/atribusi pada `users`, dan penyesuaian kebijakan RLS lookup email
  kontak CRM.
- `000190_seed_social_claim_invite_template`: template notifikasi `social.customer_claim_invite`.

Setiap migration punya pasangan `.up.sql`/`.down.sql`.

## Di luar Tahap 1 (frontend)

Halaman web `/claim?token=`, layar untuk user tanpa membership, tombol undang/tautkan di detail kontak CRM,
pengaturan klaim otomatis, halaman `/r/<kode>` yang menyimpan first-touch, serta pengiriman `intent=social`
dan `attribution` dari aplikasi mobile.
