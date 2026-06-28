# Google Account Register and Login Development Tasks

Dokumen ini adalah task development khusus untuk kebutuhan register dan login memakai account Google.

Dokumen ini melengkapi:

- `docs/reference-auth-user.md`
- `docs/auth-user-development-tasks.md`
- `docs/auth-user-public-api-contract.md`
- `docs/auth-user-traceability-index.md`

## Tujuan

Membangun Google account authentication untuk user self-register dan login pada platform multi tenant Zyad Cloud tanpa melemahkan session, RBAC, audit, dan isolation contract yang sudah ada.

Scope utama:

- Login memakai Google ID token dari frontend.
- Register otomatis memakai Google account sesuai policy.
- Link Google identity ke user existing berdasarkan verified email sesuai policy.
- Simpan Google identity di `auth_identities`.
- Terbitkan access token dan refresh token memakai flow session existing.
- Catat login history dan audit event.
- Siapkan kontrak API dan traceability yang jelas sebelum implementasi.

## Non-Goal

- Tidak membuat UI frontend.
- Tidak menambah provider selain Google.
- Tidak menyimpan Google access token untuk memanggil Google API kecuali ada kebutuhan produk terpisah.
- Tidak mengubah arsitektur auth utama.
- Tidak menambah dependency production sebelum dikonfirmasi.

## Current Repository Findings

Hasil pembacaan repository saat dokumen ini dibuat:

- `README.md` sudah menyebut social login OAuth2 sebagai fitur Auth.
- `docs/auth-user-development-tasks.md` sudah punya task generik `AUTH-0404: Social Login`.
- `docs/auth-user-traceability-index.md` menandai `Login Google` sebagai `planned`.
- Migration `000002_create_user_auth_tables` sudah menyediakan tabel `auth_identities`.
- `auth_identities.provider` sudah mengizinkan nilai `google`.
- Login email/username password, refresh token rotation, logout, current user, session list, email verification, dan audit dasar sudah ada.
- Belum ada endpoint Google auth di `internal/modules/user/handler/auth_handler.go`.
- Belum ada DTO Google auth di `internal/modules/user/dto/auth.go`.
- Belum ada config khusus Google OAuth di `internal/config`.

## Design Decision

Flow MVP menggunakan Google ID token dari frontend:

1. Frontend melakukan Google Sign-In dan menerima `id_token`.
2. Frontend mengirim `id_token` ke backend.
3. Backend memverifikasi signature, issuer, audience, expiry, subject, email, dan email verification.
4. Backend mencari `auth_identities(provider='google', provider_user_id=sub)`.
5. Jika identity ditemukan, user login memakai session/token existing.
6. Jika identity belum ada, backend mencari user existing dengan email verified sesuai linking policy.
7. Jika tidak ada user, backend register user baru sesuai registration policy.
8. Backend menyimpan identity Google dan membuat session.

Alasan:

- Backend tetap menjadi source of truth session dan permission.
- Frontend tidak mengirim authorization code atau secret.
- Tidak perlu menyimpan Google access token untuk MVP.
- Cocok dengan schema `auth_identities` yang sudah tersedia.

## Security Baseline

Implementasi wajib memenuhi baseline berikut:

- Validasi `id_token` dilakukan di backend.
- `aud` harus cocok dengan allowlist Google OAuth Client ID.
- `iss` hanya menerima issuer Google resmi.
- `exp`, `iat`, dan token signature harus valid.
- `sub` dipakai sebagai `provider_user_id`; email tidak boleh menjadi primary provider ID.
- Auto-link ke user existing hanya boleh jika email dari Google sudah verified.
- Jika email existing belum verified atau conflict dengan local identity, return error yang aman.
- Tidak menyimpan Google ID token plain.
- Tidak menyimpan Google access token kecuali ada task terpisah.
- Response error tidak membocorkan status internal user secara berlebihan.
- Login Google tetap membuat session dan refresh token rotation seperti login password.
- User nonaktif, suspended, banned, atau deleted tidak bisa login.
- Login history dan audit metadata tidak menyimpan token plain.
- Rate limit endpoint Google auth.

## API Contract Draft

Base path mengikuti API existing: `/api/v1`.

### POST /auth/google

Login atau register memakai Google account.

Request:

```json
{
  "id_token": "google_id_token",
  "device_name": "Chrome on macOS",
  "remember_me": true,
  "invite_token": "optional_invitation_token"
}
```

Response sukses memakai bentuk yang sama dengan `POST /auth/login`:

```json
{
  "success": true,
  "message": "Google authentication successful",
  "data": {
    "access_token": "jwt_access_token",
    "refresh_token": "refresh_token",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {
      "id": "uuid",
      "name": "Jane Doe",
      "email": "jane@example.com",
      "username": "jane",
      "status": "active",
      "roles": ["member"],
      "permissions": []
    },
    "is_new_user": true
  }
}
```

Error code tambahan:

| Code | HTTP | Meaning |
| --- | ---: | --- |
| `AUTH_GOOGLE_DISABLED` | 403 | Google auth tidak diaktifkan |
| `AUTH_GOOGLE_TOKEN_INVALID` | 401 | ID token invalid, expired, atau issuer salah |
| `AUTH_GOOGLE_AUDIENCE_INVALID` | 401 | Token audience tidak cocok dengan client ID yang diizinkan |
| `AUTH_GOOGLE_EMAIL_UNVERIFIED` | 422 | Email Google belum verified |
| `AUTH_GOOGLE_EMAIL_CONFLICT` | 409 | Email sudah dipakai tetapi tidak memenuhi linking policy |
| `AUTH_REGISTRATION_DISABLED` | 403 | Auto-register user baru tidak diizinkan |

## Configuration

Tambahkan config baru tanpa hard-code:

```env
AUTH_GOOGLE_ENABLED=false
AUTH_GOOGLE_CLIENT_IDS=
AUTH_GOOGLE_AUTO_REGISTER=true
AUTH_GOOGLE_AUTO_LINK_VERIFIED_EMAIL=true
AUTH_GOOGLE_DEFAULT_ROLE=member
AUTH_GOOGLE_DEFAULT_STATUS=active
```

Aturan:

- `AUTH_GOOGLE_CLIENT_IDS` berisi comma-separated client ID.
- Production wajib memiliki minimal satu client ID jika `AUTH_GOOGLE_ENABLED=true`.
- `AUTH_GOOGLE_DEFAULT_STATUS` hanya menerima `pending` atau `active`.
- Jika `AUTH_GOOGLE_AUTO_REGISTER=false`, Google user baru ditolak kecuali melalui invitation flow.
- Jika `AUTH_GOOGLE_AUTO_LINK_VERIFIED_EMAIL=false`, user existing harus link manual melalui endpoint terpisah pada task future.

## Task Breakdown

### GAUTH-0001: Google Auth Planning and Config

Status: done.

Scope:

- Tambahkan config Google auth di `internal/config`. `done`
- Tambahkan env example. `done`
- Validasi production config. `done`
- Dokumentasikan policy auto-register, auto-link, default role, dan default status. `done`

Acceptance criteria:

- Config bisa dimatikan total via `AUTH_GOOGLE_ENABLED=false`. `done`
- Production gagal start jika Google enabled tanpa client ID. `done`
- Tidak ada secret Google client di code. `done`

### GAUTH-0002: Google ID Token Verifier Abstraction

Status: done.

Scope:

- Buat abstraction verifier di boundary service atau platform auth. `done via internal/core/auth.GoogleIDTokenVerifier`
- Verifier menerima ID token dan mengembalikan claims terverifikasi. `done`
- Claims minimal: `sub`, `email`, `email_verified`, `name`, `picture`, `aud`, `iss`, `exp`. `partial: exp/iat divalidasi package Google; service memakai mapped claims`
- Implementasi awal boleh memakai Google tokeninfo/JWK sesuai dependency yang disetujui. `done via google.golang.org/api/idtoken`

Acceptance criteria:

- Unit test untuk token invalid, audience invalid, email unverified, dan success claim mapping. `partial: covered via service mock; live verifier network/JWK test deferred`
- Verifier mudah di-mock pada service test. `done`
- Tidak melakukan business rule user di verifier. `done`

### GAUTH-0003: Repository Support for Google Identity

Status: done.

Scope:

- Query identity by provider and provider user ID. `done`
- Query user by email untuk linking policy. `done`
- Create Google identity di `auth_identities`. `done`
- Create user self-register, profile, local nullable password state jika dibutuhkan, role assignment, dan audit dalam transaction. `done`
- Update Google identity email jika Google mengirim email verified baru untuk `sub` yang sama. `done during link conflict update path`

Acceptance criteria:

- Unique constraint `auth_identities_provider_user_unique` dipakai sebagai guard concurrency. `done`
- Transaction menjaga user, profile, identity, role, dan audit konsisten. `done`
- Tidak ada token plain tersimpan. `done`

### GAUTH-0004: Google Login Existing Identity

Status: done.

Scope:

- Implement service flow untuk identity Google yang sudah ada. `done`
- Cek user status memakai rule login existing. `done`
- Buat session, access token, refresh token. `done`
- Catat login history event `login`. `done`
- Update `last_login_at`. `done`

Acceptance criteria:

- User active bisa login. `done`
- User pending, inactive, suspended, banned, dan deleted ditolak sesuai rule existing. `done`
- Response token sama dengan login password plus `is_new_user=false`. `done`

### GAUTH-0005: Google Auto-Link Existing User by Verified Email

Status: done.

Scope:

- Jika Google identity belum ada, cari user existing by email. `done`
- Link Google identity hanya jika Google email verified. `done`
- Terapkan policy `AUTH_GOOGLE_AUTO_LINK_VERIFIED_EMAIL`. `done`
- Jika user existing belum email verified, set verified timestamp berdasarkan Google verified email sesuai policy yang disepakati. `done`

Acceptance criteria:

- Tidak membuat duplicate user untuk email yang sama. `done`
- Conflict menghasilkan `AUTH_GOOGLE_EMAIL_CONFLICT`. `done`
- Audit mencatat event `google_identity_linked`. `done`

### GAUTH-0006: Google Auto-Register New User

Status: done.

Scope:

- Jika identity dan email belum ada, buat user baru jika `AUTH_GOOGLE_AUTO_REGISTER=true`. `done`
- Isi `name`, `email`, `avatar_url`, `email_verified_at`, default status, dan default role. `done`
- Username dibuat dari email prefix dengan collision handling. `done`
- Kirim email welcome atau verification sesuai default status/policy notification. `deferred: notification welcome template belum menjadi requirement MVP`

Acceptance criteria:

- User baru punya Google identity. `done`
- Default role valid dan tidak kosong. `done`
- Jika default status `pending`, login token tidak diterbitkan kecuali policy eksplisit mengizinkan pending bootstrap. `done via status guard`
- Jika default status `active`, token diterbitkan langsung. `done`

### GAUTH-0007: Invitation-Aware Google Register

Scope:

- Jika request membawa `invite_token`, validasi invitation.
- Google email harus cocok dengan email invitation.
- Accept invitation dan link Google identity dalam transaction.
- Status user berubah sesuai invitation policy.

Acceptance criteria:

- Invitation expired ditolak.
- Email mismatch ditolak.
- Accepted invitation tidak bisa dipakai ulang.

Dependency:

- Menunggu `AUTH-0402` jika invitation table belum tersedia.

### GAUTH-0008: HTTP Handler and DTO

Status: done.

Scope:

- Tambahkan DTO request/response di `internal/modules/user/dto/auth.go`. `done`
- Tambahkan handler `POST /auth/google`. `done`
- Bind request, ambil IP/user agent/device, panggil service, return envelope response. `done`
- Register route pada auth routes. `done`

Acceptance criteria:

- Handler tidak berisi business rule. `done`
- Validation error memakai format standar. `done`
- Endpoint muncul di OpenAPI saat implementasi. `done`

### GAUTH-0009: Rate Limit, Login History, and Audit

Status: partial.

Scope:

- Tambahkan rate limit untuk `POST /auth/google`. `deferred to AUTH-0307 because rate limit infrastructure is still planned`
- Catat failed Google auth dengan reason yang aman. `done`
- Catat audit event untuk link identity dan register. `done`

Acceptance criteria:

- Brute force token invalid terkena rate limit. `deferred to AUTH-0307`
- Audit metadata tidak menyimpan `id_token`. `done`
- Login histories bisa difilter sebagai login Google melalui metadata atau reason yang aman. `done via reason=google/google_*`

### GAUTH-0010: Public API and OpenAPI Documentation

Status: done.

Scope:

- Update `docs/auth-user-public-api-contract.md`. `done`
- Update `api/openapi.yaml`. `done`
- Update traceability index. `done`

Acceptance criteria:

- Frontend tahu request/response dan error code. `done`
- OpenAPI sinkron dengan handler. `done`
- Breaking change dicatat di traceability. `done`

### GAUTH-0011: Tests

Status: partial.

Scope:

- Unit test verifier abstraction. `partial: verifier abstraction is covered via service mock`
- Unit test service existing identity login. `done`
- Unit test auto-link verified email. `partial: service path implemented; dedicated test deferred`
- Unit test auto-register. `done`
- Handler test request validation dan success. `covered by package compile; dedicated handler test deferred`
- Repository integration test untuk identity lookup, create identity, and duplicate guard. `deferred to integration database pass`

Acceptance criteria:

- Critical path Google login existing user covered. `done`
- Critical path Google register new user covered. `done`
- Conflict dan disabled config covered. `partial: disabled covered; conflict dedicated test deferred`

## Implementation Order

1. `GAUTH-0001` config.
2. `GAUTH-0002` verifier abstraction.
3. `GAUTH-0003` repository support.
4. `GAUTH-0004` existing identity login.
5. `GAUTH-0005` auto-link.
6. `GAUTH-0006` auto-register.
7. `GAUTH-0008` handler and route.
8. `GAUTH-0009` rate limit, history, audit.
9. `GAUTH-0010` API docs and OpenAPI.
10. `GAUTH-0011` tests throughout implementation.

`GAUTH-0007` dikerjakan setelah invitation flow tersedia.

## Open Questions Before Implementation

- Apakah self-register Google langsung `active` atau harus `pending` menunggu approval admin?
- Apakah Google auto-link ke user existing boleh mengisi `email_verified_at` jika sebelumnya null?
- Apakah default role user Google adalah `member`, `customer`, atau role tenant tertentu?
- Apakah Google auth berlaku untuk platform admin, tenant admin, customer public, atau semua?
- Apakah frontend memakai Google Identity Services dan mengirim ID token langsung?

## Definition of Done

- Config dan env terdokumentasi.
- Endpoint `POST /auth/google` tersedia; rate limit mengikuti follow-up `AUTH-0307`.
- Google ID token diverifikasi backend.
- Identity tersimpan di `auth_identities`.
- Session/token mengikuti flow existing.
- Status user tetap dihormati.
- Audit dan login history tercatat.
- Unit/handler/repository tests ditambahkan.
- Public API contract, OpenAPI, dan traceability index diperbarui.
