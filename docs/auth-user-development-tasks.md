# Auth and User Development Tasks

Dokumen ini adalah task development untuk module:

- `internal/core/auth`
- `internal/modules/user`

Sumber requirement utama:

- `docs/reference-auth-user.md`
- `README.md`
- `docs/migration-guide.md`

## Tujuan

Membangun fondasi Auth dan User Management untuk CRM enterprise dan platform multi tenant. Scope meliputi login, token, session, reset password, email/phone verification, user CRUD, profile, status, role assignment, permission assignment, audit log, dan dokumentasi API publik.

## Catatan Konflik Dokumentasi

Request development meminta seed super admin mengambil `username`, `email`, dan `password` dari `.env`.

Kondisi awal `.env.example` saat dokumen ini dibuat:

- Sudah ada `SEED_ADMIN_NAME`
- Sudah ada `SEED_ADMIN_EMAIL`
- Sudah ada `SEED_ADMIN_WHATSAPP`
- Sudah ada `SEED_ADMIN_ROLE`
- Belum ada `SEED_ADMIN_USERNAME`
- Belum ada `SEED_ADMIN_PASSWORD`

Status: resolved pada task awal `AUTH-0001` dengan menambahkan `SEED_ADMIN_USERNAME`, `SEED_ADMIN_PASSWORD`, dan loader `SeedAdminConfig`.

## Development Pattern

### Auth Core

`internal/core/auth` berisi helper dan komponen teknis lintas module.

Tanggung jawab:

- Password hashing dan password verification.
- JWT access token dan refresh token.
- Claims dan current user context.
- Token hash helper untuk refresh token, reset token, verification token, dan OTP.
- Auth middleware helper yang dipakai `internal/core/middleware`.
- Session identity helper.

Tidak boleh berisi:

- HTTP handler langsung.
- Query database module user.
- Business rule user management.

### User Module

`internal/modules/user` mengikuti layer:

- `handler`: HTTP binding, auth context, validasi request, response.
- `service`: business rule, transaction boundary, idempotency, integrasi module.
- `repository`: query database saja.
- `dto`: request dan response contract.
- `model`: entity dan enum domain.
- `routes.go`: route registration.
- `errors.go`: domain error module user.

### Boundary Antar Module

- Auth login membaca credential user melalui service/repository user.
- User service memegang status, profile, role assignment, dan permission assignment.
- Permission check memakai `internal/core/permission`.
- Middleware berada di `internal/core/middleware`.
- Email, WhatsApp, Redis, dan storage memakai adapter di `internal/platform`.

## Security Baseline

Semua task Auth/User wajib memenuhi baseline berikut:

- Password di-hash dengan bcrypt atau argon2.
- Password plain tidak pernah disimpan.
- Refresh token disimpan sebagai hash.
- Reset token disimpan sebagai hash.
- Email verification token disimpan sebagai hash.
- OTP disimpan sebagai hash.
- Token plain hanya muncul sekali di response atau channel notifikasi.
- Response API tidak expose `password_hash`.
- Login, forgot password, OTP, dan resend verification wajib rate limited.
- Aksi sensitif wajib membuat audit log.
- Logout dan revoke session harus mencabut refresh token.
- CORS memakai domain frontend yang valid dari config.
- `APP_FRONTEND_URL` dan `APP_URL` dipisah.

## Phase 0 - Foundation

### AUTH-0001: Auth Config

Scope:

- Tambahkan config auth yang dibutuhkan untuk access token, refresh token, reset token, verification token, OTP, password policy, dan session.
- Pastikan env dipetakan melalui `internal/config`.
- Tambahkan env seed super admin:
  - `SEED_ADMIN_USERNAME`
  - `SEED_ADMIN_EMAIL`
  - `SEED_ADMIN_PASSWORD`
  - `SEED_ADMIN_NAME`
  - `SEED_ADMIN_WHATSAPP`
  - `SEED_ADMIN_ROLE`

Acceptance criteria:

- Config punya fallback aman untuk development. `done`
- Secret wajib divalidasi saat production. `done`
- `.env.example` terdokumentasi. `done`

### AUTH-0002: Auth Helper

Scope:

- Implement password hasher. `done`
- Implement token generator dan parser. `done`
- Implement refresh token hash. `done`
- Implement random token untuk reset, verification, invitation, dan OTP. `done`
- Implement claims model. `done`

Acceptance criteria:

- Unit test untuk hash password, verify password, token parse, expired token, dan invalid signature. `done`

### USER-0001: User Domain Model

Scope:

- Definisikan model `User`, `UserProfile`, `UserStatus`, `Role`, `Permission`, `UserRole`, `UserPermission`, `Session`, `LoginHistory`, dan `AuditLog`. `done`
- Definisikan enum status:
  - `active` `done`
  - `inactive` `done`
  - `pending` `done`
  - `suspended` `done`
  - `banned` `done`
  - `deleted` `done`
  - `invited` `done`

Acceptance criteria:

- Field model selaras dengan migration. `done`
- Soft delete memakai `deleted_at`. `done`
- Status invalid ditolak di service. `pending service implementation`

### USER-0002: Database Migration Plan Implementation

Scope:

- Buat migration untuk tabel users dan auth. `done`
- Buat migration cleanup jika perlu menyesuaikan tabel `roles`, `permissions`, `role_permissions`, dan `user_roles` yang sudah ada. `done`
- Jangan ubah migration lama yang sudah ada. `done`

Reference:

- `docs/auth-user-migration-seed-plan.md`

Acceptance criteria:

- Semua migration punya pasangan `up` dan `down`. `done`
- Rollback bisa dilakukan minimal satu step. `done`
- Schema memakai `public`. `done`

## Phase 1 - Auth Basic

### AUTH-0101: Login Email or Username Password

Scope:

- `POST /auth/login` `done`
- Login memakai `email` atau `username`. `done`
- Validasi password. `done`
- Cek status user. `done`
- Catat login success dan failed login. `done`
- Buat access token dan refresh token. `done`
- Simpan session dengan device, user agent, IP, expiry, dan refresh token hash. `done`
- Support `remember_me` untuk masa refresh token lebih panjang. `done`

Acceptance criteria:

- User `active` bisa login. `done`
- User `pending`, `inactive`, `suspended`, `banned`, dan `deleted` ditolak dengan error jelas. `done`
- Failed login menaikkan counter throttling. `partial: failed login dicatat di login_histories; rate limit counter eksplisit masuk AUTH-0307`
- Login success reset throttling. `deferred to AUTH-0307`

### AUTH-0102: Logout

Scope:

- `POST /auth/logout` `done`
- Revoke current session. `done`
- Revoke current refresh token. `done via session revoke; refresh token hash remains unusable once session is revoked`
- Catat audit log. `done via login_histories logout/token_revoked event; audit_logs detail masuk USER-0301`

Acceptance criteria:

- Refresh token lama tidak bisa dipakai setelah logout. `done for session state; enforced by refresh flow in AUTH-0103`
- Access token tetap short lived. `done`

### AUTH-0103: Refresh Token Rotation

Scope:

- `POST /auth/refresh-token` `done`
- Validasi refresh token. `done`
- Cocokkan hash token dengan session aktif. `done`
- Rotate refresh token setiap digunakan. `done`
- Update `last_used_at`. `done`

Acceptance criteria:

- Refresh token lama invalid setelah dipakai. `done`
- Reuse refresh token lama memicu revoke session terkait. `done`

### AUTH-0104: Current User

Scope:

- `GET /auth/me` `done`
- Return identity, profile, roles, permissions efektif, current organization jika ada, dan session metadata minimal. `done`

Acceptance criteria:

- Tidak expose credential dan token. `done`
- Response dipakai sebagai bootstrap frontend. `done`

### AUTH-0105: Forgot Password

Scope:

- `POST /auth/forgot-password`
- Generate reset token sekali pakai.
- Simpan hash token.
- Kirim link reset memakai `APP_FRONTEND_URL`.
- Rate limit per email dan IP.

Acceptance criteria:

- Response tidak membocorkan apakah email terdaftar.
- Token punya expiry.

### AUTH-0106: Validate Reset Token

Scope:

- `POST /auth/reset-password/validate`
- Validasi token belum expired dan belum used.

Acceptance criteria:

- Frontend bisa menentukan apakah halaman reset boleh lanjut.

### AUTH-0107: Reset Password

Scope:

- `POST /auth/reset-password`
- Validasi token.
- Update password hash.
- Tandai token used.
- Revoke semua session user.
- Kirim notifikasi password changed.

Acceptance criteria:

- Token tidak bisa dipakai ulang.
- Password lama tidak lagi valid.

### AUTH-0108: Change Password

Scope:

- `POST /auth/change-password`
- Butuh user login.
- Require current password.
- Terapkan password policy.
- Opsional force logout other devices.

Acceptance criteria:

- Current session tetap aktif jika policy mengizinkan.
- Device lain bisa dicabut sesuai request.

## Phase 2 - User Management and RBAC

### USER-0201: Admin List Users

Scope:

- `GET /admin/users`
- Filter by status, search, role, organization, created date, deleted state.
- Pagination dan sorting.

Acceptance criteria:

- Response memakai metadata pagination.
- Query tidak mengembalikan `password_hash`.

### USER-0202: Admin Detail User

Scope:

- `GET /admin/users/:id`
- Return user, profile, roles, direct permissions, sessions summary, dan audit summary.

Acceptance criteria:

- 404 untuk user tidak ditemukan.
- Deleted user hanya terlihat jika filter/permission mengizinkan.

### USER-0203: Admin Create User

Scope:

- `POST /admin/users`
- Admin membuat user dengan email, username, phone, profile, status, roles, dan optional temporary password.
- Jika password tidak dikirim, generate invite atau reset link.

Acceptance criteria:

- Email dan username unik.
- Role assignment valid.
- Audit log tercatat.

### USER-0204: Admin Update User

Scope:

- `PATCH /admin/users/:id`
- Update identity aman dan profile.
- Email/phone change dapat mengubah verified state sesuai policy.

Acceptance criteria:

- Partial update.
- Field sensitif punya permission khusus.

### USER-0205: Soft Delete and Restore User

Scope:

- `DELETE /admin/users/:id`
- `POST /admin/users/:id/restore`
- Soft delete user.
- Revoke session saat delete.

Acceptance criteria:

- User soft deleted tidak bisa login.
- Restore mengembalikan status sesuai policy.

### USER-0206: Bulk Action

Scope:

- `POST /admin/users/bulk-action`
- Bulk delete dan bulk update status.

Acceptance criteria:

- Validasi jumlah maksimal item per request.
- Partial failure dilaporkan secara terstruktur.

### USER-0207: User Status Management

Scope:

- `PATCH /admin/users/:id/status`
- `POST /admin/users/:id/activate`
- `POST /admin/users/:id/suspend`
- `POST /admin/users/:id/ban`
- Simpan alasan status untuk suspend dan ban.

Acceptance criteria:

- Suspended dan banned user tidak bisa login.
- Audit log mencatat actor, status lama, status baru, dan alasan.

### USER-0208: Self Profile

Scope:

- `GET /users/me`
- `PATCH /users/me/profile`
- `PATCH /users/me/avatar`
- Update name, phone, avatar, bio, job title, department, address, timezone, language preference.

Acceptance criteria:

- User hanya bisa update profile sendiri.
- Avatar memakai storage adapter jika upload file diaktifkan.

### USER-0209: Role Assignment

Scope:

- `GET /admin/users/:id/roles`
- `POST /admin/users/:id/roles`
- `DELETE /admin/users/:id/roles/:roleId`
- Support multiple roles.
- Support organization scoped role.

Acceptance criteria:

- Role tidak bisa duplicate untuk scope yang sama.
- Super admin role dilindungi dari penghapusan sembarang.

### USER-0210: Permission Assignment

Scope:

- `GET /admin/users/:id/permissions`
- `POST /admin/users/:id/permissions`
- `DELETE /admin/users/:id/permissions/:permissionId`
- Support direct allow dan deny permission.
- Hitung effective permission dari role plus override.

Acceptance criteria:

- Deny permission mengalahkan allow permission.
- Permission cache invalidated setelah perubahan.

### USER-0211: Role API

Scope:

- `GET /admin/roles`
- `GET /admin/roles/:id`
- `POST /admin/roles`
- `PATCH /admin/roles/:id`
- `DELETE /admin/roles/:id`
- `GET /admin/roles/:id/permissions`
- `POST /admin/roles/:id/permissions`
- `DELETE /admin/roles/:id/permissions/:permissionId`

Acceptance criteria:

- System role tidak bisa dihapus.
- Role slug unik.

### USER-0212: Permission API

Scope:

- `GET /admin/permissions`
- `GET /admin/permissions/grouped`
- `GET /admin/permission-matrix`

Acceptance criteria:

- Permission grouped by module.
- Permission matrix bisa dipakai UI admin.

### USER-0213: RBAC Middleware Integration

Scope:

- Auth middleware membaca claims.
- Active user middleware cek status.
- Permission middleware cek permission efektif.
- Role middleware tersedia untuk guard role khusus.

Acceptance criteria:

- Endpoint admin dilindungi permission.
- Error forbidden konsisten.

## Phase 3 - Security, Session, Verification, Audit

### AUTH-0301: Session List

Scope:

- `GET /auth/sessions`
- User melihat session aktif miliknya.

Acceptance criteria:

- Current session ditandai.
- Data device/IP tidak terlalu detail untuk menghindari kebocoran.

### AUTH-0302: Revoke Session

Scope:

- `DELETE /auth/sessions/:id`
- `DELETE /admin/users/:id/sessions/:sessionId`
- User atau admin mencabut session.

Acceptance criteria:

- Session revoked tidak bisa refresh token.
- Audit log tercatat.

### AUTH-0303: Logout All

Scope:

- `POST /auth/logout-all`
- Revoke semua session user.

Acceptance criteria:

- Bisa exclude current session jika dibutuhkan oleh request.

### AUTH-0304: Email Verification

Scope:

- `POST /auth/verify-email`
- `POST /auth/resend-verification-email`
- Generate token hash.
- Kirim link verification memakai `APP_FRONTEND_URL`.

Acceptance criteria:

- Token expired tidak valid.
- Email verified mengisi `email_verified_at`.

### AUTH-0305: Phone or WhatsApp Verification

Scope:

- OTP untuk verification phone.
- Simpan code hash, purpose, destination, expiry, attempts.

Acceptance criteria:

- OTP rate limited.
- Attempts maksimal diterapkan.

### AUTH-0306: Login History

Scope:

- Catat success login, failed login, logout, token refresh, dan revoked session.
- Simpan IP, user agent, device, waktu, dan reason.

Acceptance criteria:

- Admin bisa melihat riwayat user.

### USER-0301: Audit Log

Scope:

- `GET /admin/users/:id/audit-logs`
- `GET /admin/audit-logs?module=user`
- Catat aksi:
  - login
  - logout
  - failed login
  - password changed
  - role assigned
  - role removed
  - permission changed
  - user suspended
  - user banned
  - profile updated
  - token revoked

Acceptance criteria:

- Audit log menyimpan actor, target, event, metadata, IP, user agent, created at.
- Metadata tidak menyimpan token plain atau password.

### AUTH-0307: Rate Limit and Captcha Hook

Scope:

- Rate limit login, forgot password, resend verification, OTP request, dan OTP verify.
- Captcha required setelah gagal beberapa kali.

Acceptance criteria:

- Bisa memakai Redis jika tersedia.
- Jika captcha belum diimplementasikan, service expose flag `captcha_required`.

## Phase 4 - Enterprise Extension

### AUTH-0401: Register Manual

Scope:

- `POST /auth/register`
- Create self registered user.
- Default role dari config atau database.
- Status awal `pending` atau `active` sesuai policy.

Acceptance criteria:

- Email verification dikirim setelah register.
- Duplicate email/username ditolak.

### AUTH-0402: Invitation

Scope:

- Admin invite user via email.
- `POST /auth/invite/accept`
- Invitation token hash.
- Status user `invited` sampai accepted.

Acceptance criteria:

- Invitation expired tidak valid.
- Accepted invitation membuat password dan mengaktifkan user sesuai policy.

### AUTH-0403: WhatsApp OTP Login

Scope:

- `POST /auth/login/whatsapp/request-otp`
- `POST /auth/login/whatsapp/verify-otp`
- Integrasi `internal/platform/whatsapp`.

Acceptance criteria:

- OTP hashed.
- Attempts dan expiry diterapkan.

### AUTH-0404: Social Login

Scope:

- `POST /auth/google`
- `POST /auth/github`
- Tambahkan provider GitLab dan Facebook jika dibutuhkan.
- Simpan identity di `auth_identities`.

Acceptance criteria:

- Provider identity unik.
- Link ke user existing berdasarkan verified email sesuai policy.

### AUTH-0405: Two Factor Authentication

Scope:

- `POST /auth/2fa/setup`
- `POST /auth/2fa/verify`
- `POST /auth/2fa/disable`
- `GET /auth/2fa/recovery-codes`
- Support TOTP, email/WhatsApp OTP, dan recovery codes.

Acceptance criteria:

- Recovery code disimpan sebagai hash.
- Disable 2FA butuh current password atau step-up verification.

### USER-0401: Organization Support

Scope:

- `GET /users/me/organizations`
- `POST /users/me/switch-organization`
- `GET /admin/organizations/:id/users`
- `POST /admin/organizations/:id/users`
- Role per organization.
- Branch dan department.

Acceptance criteria:

- Data access scoped by organization.
- Current organization ada di auth context.

### USER-0402: Import Export

Scope:

- Import users via CSV/XLSX jika dibutuhkan.
- Export users dengan filter.

Acceptance criteria:

- Import dry run.
- Error row level.
- Export tidak menyertakan data rahasia.

## Public API Documentation Task

### DOC-API-0001: Public API Contract

Scope:

- Buat dan maintain kontrak API publik untuk frontend dan aplikasi eksternal.
- Endpoint, request, response, auth scheme, error code, pagination, rate limit, dan versioning harus terdokumentasi.
- Sinkronkan dengan `api/openapi.yaml` jika file tersebut tersedia.

Reference:

- `docs/auth-user-public-api-contract.md`

Acceptance criteria:

- Frontend bisa consume API tanpa membaca source code backend.
- Breaking change dicatat di traceability index.

## Testing Strategy

Minimal test:

- Unit test auth helper.
- Unit test user service rule.
- Repository test untuk query penting jika test database tersedia.
- Handler test untuk endpoint critical path.
- Migration up/down test pada database lokal atau container.

Critical path test:

- Login success.
- Login failed.
- Refresh token rotation.
- Logout.
- Forgot password.
- Reset password.
- Change password.
- Admin create user.
- Admin update status.
- Assign role.
- Assign permission.
- Revoke session.

## Definition of Done

Setiap task dianggap selesai jika:

- Code mengikuti layer module.
- Migration tersedia jika ada perubahan database.
- Response API mengikuti public contract.
- Error memakai format standar.
- Security baseline terpenuhi.
- Test relevan ditambahkan.
- Dokumentasi traceability diperbarui.
