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

- `POST /auth/forgot-password` `done`
- Generate reset token sekali pakai. `done`
- Simpan hash token. `done`
- Kirim link reset memakai `APP_FRONTEND_URL`. `done via notification outbox auth.password_reset_requested`
- Rate limit per email dan IP. `deferred to AUTH-0307`

Acceptance criteria:

- Response tidak membocorkan apakah email terdaftar. `done`
- Token punya expiry. `done`

### AUTH-0106: Validate Reset Token

Scope:

- `POST /auth/reset-password/validate` `done`
- Validasi token belum expired dan belum used. `done`

Acceptance criteria:

- Frontend bisa menentukan apakah halaman reset boleh lanjut. `done`

### AUTH-0107: Reset Password

Scope:

- `POST /auth/reset-password` `done`
- Validasi token. `done`
- Update password hash. `done`
- Tandai token used. `done`
- Revoke semua session user. `done`
- Kirim notifikasi password changed. `done via notification outbox auth.password_changed`

Acceptance criteria:

- Token tidak bisa dipakai ulang. `done via row lock and atomic token consumption`
- Password lama tidak lagi valid. `done`

### AUTH-0108: Change Password

Scope:

- `POST /auth/change-password` `done`
- Butuh user login. `done via protected auth middleware`
- Require current password. `done`
- Terapkan password policy. `done via AUTH_PASSWORD_MIN_LENGTH`
- Opsional force logout other devices. `done`

Acceptance criteria:

- Current session tetap aktif jika policy mengizinkan. `done`
- Device lain bisa dicabut sesuai request. `done`

Notes:

- Password history tetap `deferred`; tidak ada tabel atau dependency baru pada task ini.
- Perubahan password mencatat audit event `password_changed`.
- Notifikasi memakai event outbox `auth.password_changed` secara best effort.

## Phase 2 - User Management and RBAC

### USER-0201: Admin List Users

Scope:

- `GET /admin/users` `done`
- Filter by status, search, role, organization, created date, deleted state. `done`
- Pagination dan sorting. `done`

Acceptance criteria:

- Response memakai metadata pagination. `done`
- Query tidak mengembalikan `password_hash`. `done`

Notes:

- Endpoint membutuhkan permission `user.read`.
- `include_deleted=true` membutuhkan permission tambahan `user.restore`.
- Sorting dibatasi ke allowlist `name`, `email`, `status`, `created_at`, dan `updated_at`.

### USER-0202: Admin Detail User

Scope:

- `GET /admin/users/:id` `done`
- Return user, profile, roles, direct permissions, sessions summary, dan audit summary. `done`

Acceptance criteria:

- 404 untuk user tidak ditemukan. `done`
- Deleted user hanya terlihat jika filter/permission mengizinkan. `done`

Notes:

- Endpoint membutuhkan permission `user.read`.
- Soft-deleted user hanya dapat dilihat melalui `include_deleted=true` dengan permission tambahan `user.restore`.
- `direct_permissions` tidak mencakup effective permission yang berasal dari role.

### USER-0203: Admin Create User

Scope:

- `POST /admin/users` `done`
- Admin membuat user dengan email, username, phone, profile, status, roles, dan optional temporary password. `done`
- Jika password tidak dikirim, generate invite atau reset link. `done`

Acceptance criteria:

- Email dan username unik. `done`
- Role assignment valid. `done`
- Audit log tercatat. `done`

Notes:

- Endpoint membutuhkan permission `user.create`.
- Pembuatan user, profile, local identity, role, setup token, dan audit dilakukan dalam satu transaction.
- User tanpa password memakai `password_reset_tokens` sebagai one-time setup link dan event outbox `user.invited`; tabel invitation khusus tetap menjadi scope `AUTH-0402`.

### USER-0204: Admin Update User

Scope:

- `PATCH /admin/users/:id` `done`
- Update identity aman dan profile. `done`
- Email/phone change dapat mengubah verified state sesuai policy. `done`

Acceptance criteria:

- Partial update. `done`
- Field sensitif punya permission khusus. `done`

Notes:

- Endpoint membutuhkan permission `user.update`.
- Perubahan email atau phone mereset verified timestamp masing-masing.
- Perubahan status ditolak dan harus menggunakan endpoint status dengan permission `user.update_status` pada USER-0207.
- Identity, local auth identity, profile, dan audit event `user_updated` diperbarui dalam satu transaction.

### USER-0205: Soft Delete and Restore User

Scope:

- `DELETE /admin/users/:id` `done`
- `POST /admin/users/:id/restore` `done`
- Soft delete user. `done`
- Revoke session saat delete. `done`

Acceptance criteria:

- User soft deleted tidak bisa login. `done`
- Restore mengembalikan status sesuai policy. `done`

Notes:

- Delete membutuhkan permission `user.delete`; restore membutuhkan `user.restore`.
- Delete mengubah status ke `deleted`, mengisi `deleted_at`, serta mencabut session dan refresh token dalam satu transaction.
- Restore memakai status sebelum delete dari audit metadata; data lama tanpa metadata dipulihkan sebagai `inactive`.
- Session dan refresh token lama tetap revoked setelah restore.
- Audit event yang dicatat adalah `user_deleted` dan `user_restored`.

### USER-0206: Bulk Action

Scope:

- `POST /admin/users/bulk-action` `done`
- Bulk delete dan bulk update status. `done`

Acceptance criteria:

- Validasi jumlah maksimal item per request. `done`
- Partial failure dilaporkan secara terstruktur. `done`

Notes:

- Maksimal `100` user per request.
- Action `delete`, `restore`, dan `update_status` masing-masing membutuhkan permission `user.delete`, `user.restore`, dan `user.update_status`.
- Response HTTP `200` memuat total `succeeded`, `failed`, serta result/error per user.
- Duplicate atau invalid user ID dilaporkan sebagai kegagalan item tanpa membatalkan item lain.
- Status `suspended` dan `banned` membutuhkan reason; perubahan status mencatat audit `user_status_changed`.

### USER-0207: User Status Management

Scope:

- `PATCH /admin/users/:id/status` `done`
- `POST /admin/users/:id/activate` `done`
- `POST /admin/users/:id/suspend` `done`
- `POST /admin/users/:id/ban` `done`
- Simpan alasan status untuk suspend dan ban. `done`

Acceptance criteria:

- Suspended dan banned user tidak bisa login. `done`
- Audit log mencatat actor, status lama, status baru, dan alasan. `done`

Notes:

- Seluruh endpoint membutuhkan permission `user.update_status`.
- Generic status endpoint menerima `active`, `inactive`, `pending`, `suspended`, `banned`, dan `invited`; status `deleted` hanya melalui soft delete.
- `suspended` dan `banned` membutuhkan reason.
- Login, refresh token, dan access-token authentication selalu membaca status database sehingga user nonaktif langsung ditolak.
- Audit event `user_status_changed` menyimpan actor, previous status, new status, dan reason.

### USER-0208: Self Profile

Scope:

- `GET /users/me` `done`
- `PATCH /users/me/profile` `done`
- `PATCH /users/me/avatar` `done`
- Update name, phone, avatar, bio, job title, department, address, timezone, language preference. `done`

Acceptance criteria:

- User hanya bisa update profile sendiri. `done`
- Avatar memakai storage adapter jika upload file diaktifkan. `done`

### USER-0209: Role Assignment `done`

Scope:

- `GET /admin/users/:id/roles` `done`
- `POST /admin/users/:id/roles` `done`
- `DELETE /admin/users/:id/roles/:roleId` `done`
- Support multiple roles. `done`
- Support organization scoped role. `done`

Acceptance criteria:

- Role tidak bisa duplicate untuk scope yang sama. `done`
- Super admin role dilindungi dari penghapusan sembarang. `done`

### USER-0210: Permission Assignment `done`

Scope:

- `GET /admin/users/:id/permissions` `done`
- `POST /admin/users/:id/permissions` `done`
- `DELETE /admin/users/:id/permissions/:permissionId` `done`
- Support direct allow dan deny permission. `done`
- Hitung effective permission dari role plus override. `done`

Acceptance criteria:

- Deny permission mengalahkan allow permission. `done`
- Permission cache invalidated setelah perubahan. `partial: cache invalidation deferred to Redis phase`

### USER-0211: Role API `done`

Scope:

- `GET /admin/roles` `done`
- `GET /admin/roles/:id` `done`
- `POST /admin/roles` `done`
- `PATCH /admin/roles/:id` `done`
- `DELETE /admin/roles/:id` `done`
- `GET /admin/roles/:id/permissions` `done`
- `POST /admin/roles/:id/permissions` `done`
- `DELETE /admin/roles/:id/permissions/:permissionId` `done`

Acceptance criteria:

- System role tidak bisa dihapus. `done`
- Role slug unik. `done`

### USER-0212: Permission API `done`

Scope:

- `GET /admin/permissions` `done`
- `GET /admin/permissions/grouped` `done`
- `GET /admin/permission-matrix` `done`

Acceptance criteria:

- Permission grouped by module. `done`
- Permission matrix bisa dipakai UI admin. `done`

### USER-0213: RBAC Middleware Integration

Scope:

- Auth middleware membaca claims. `done`
- Active user middleware cek status. `done via AuthService.AuthenticateAccessToken`
- Permission middleware cek permission efektif. `done`
- Role middleware tersedia untuk guard role khusus. `done`

Acceptance criteria:

- Endpoint admin dilindungi permission.
- Error forbidden konsisten.

## Phase 3 - Security, Session, Verification, Audit

### AUTH-0301: Session List

Scope:

- `GET /auth/sessions` `done`
- User melihat session aktif miliknya. `done`

Acceptance criteria:

- Current session ditandai. `done`
- Data device/IP tidak terlalu detail untuk menghindari kebocoran. `done`

### AUTH-0302: Revoke Session

Scope:

- `DELETE /auth/sessions/:id` `done`
- `DELETE /admin/users/:id/sessions/:sessionId` `done`
- User atau admin mencabut session. `done`

Acceptance criteria:

- Session revoked tidak bisa refresh token. `done`
- Audit log tercatat. `done via session database state; detail audit log terpusat di USER-0301`

### AUTH-0303: Logout All

Scope:

- `POST /auth/logout-all` `done`
- Revoke semua session user. `done`

Acceptance criteria:

- Bisa exclude current session jika dibutuhkan oleh request. `done`

### AUTH-0304: Email Verification

Scope:

- `POST /auth/verify-email` `done`
- `POST /auth/resend-verification-email` `done`
- Generate token hash. `done`
- Kirim link verification memakai `APP_FRONTEND_URL`. `done`

Acceptance criteria:

- Token expired tidak valid. `done`
- Email verified mengisi `email_verified_at`. `done`

### AUTH-0305: Phone or WhatsApp Verification

Scope:

- OTP untuk verification phone.
- Simpan code hash, purpose, destination, expiry, attempts.

Acceptance criteria:

- OTP rate limited.
- Attempts maksimal diterapkan.

### AUTH-0306: Login History `done`

Scope:
- Catat success login, failed login, logout, token refresh, dan revoked session. `done`
- Simpan IP, user agent, device, waktu, dan reason. `done`

Acceptance criteria:
- Admin bisa melihat riwayat user. `done via GET /api/v1/admin/login-histories and GET /api/v1/admin/users/:id/login-histories`

### USER-0301: Audit Log `done`

Scope:
- `GET /admin/users/:id/audit-logs` `done`
- `GET /admin/audit-logs?module=user` `done`
- Catat aksi:
  - login `done`
  - logout `done`
  - failed login `done`
  - password changed `done`
  - role assigned `done`
  - role removed `done`
  - permission changed `done`
  - user suspended `done`
  - user banned `done`
  - profile updated `done`
  - token revoked `done`

Acceptance criteria:
- Audit log menyimpan actor, target, event, metadata, IP, user agent, created at. `done`
- Metadata tidak menyimpan token plain atau password. `done`

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

Status: in_progress.

Progress:

- `GET /users/me/organizations` and `POST /users/me/switch-organization` are implemented by `MT-CORE-006`.
- The active organization, membership, and membership version are persisted in the authenticated session and resolved into request auth context.
- Organization-scoped role IDs and slugs are included in self organization list and switch responses.
- Tenant-scoped request permission evaluation is completed by `MT-CORE-007`; tenant APIs must use its explicit organization middleware.
- Organization admin user APIs, branch, and department remain follow-up scope.

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
