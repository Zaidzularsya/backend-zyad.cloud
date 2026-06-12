# Auth and User Traceability Index

Dokumen ini menghubungkan requirement dari `docs/reference-auth-user.md` ke task development, kontrak API, migration, seed, dan status implementasi.

Status:

- `planned`: belum dibangun.
- `in_progress`: sedang dibangun.
- `done`: selesai dan sudah diverifikasi.
- `deferred`: sengaja ditunda.

## Source Documents

| Source | Purpose |
| --- | --- |
| `docs/reference-auth-user.md` | Feature reference Auth dan User |
| `docs/auth-user-development-tasks.md` | Breakdown task development |
| `docs/auth-user-public-api-contract.md` | Kontrak API untuk Frontend dan aplikasi eksternal |
| `docs/auth-user-migration-seed-plan.md` | Rencana migration dan seed |
| `docs/migration-guide.md` | Cara menjalankan migration |

## Module Mapping

| Module | Path | Responsibility |
| --- | --- | --- |
| Auth core | `internal/core/auth` | Token, password, claims, auth helper |
| User module | `internal/modules/user` | User identity, profile, role, permission, session, audit |
| Middleware | `internal/core/middleware` | Auth guard, active user, permission guard |
| Permission core | `internal/core/permission` | Permission evaluation |
| Platform mail | `internal/platform/mail` | Email notification |
| Platform WhatsApp | `internal/platform/whatsapp` | OTP and notification |
| Platform Redis | `internal/platform/redis` | Rate limit, cache, session support |

## Requirement Traceability

| Requirement | Reference Section | Task ID | API Contract | Migration/Seed | Status |
| --- | --- | --- | --- | --- | --- |
| Login email password | 1.A Login | AUTH-0101 | `POST /auth/login` | `users`, `sessions`, `login_histories` | done |
| Login username password | 1.A Login | AUTH-0101 | `POST /auth/login` | `users.username` | done |
| Login WhatsApp OTP | 1.A Login | AUTH-0403 | WhatsApp OTP endpoints | `otp_codes` | planned |
| Login Google | 1.A Login | AUTH-0404 | `POST /auth/google` | `auth_identities` | planned |
| Login GitHub/GitLab | 1.A Login | AUTH-0404 | Social login endpoints | `auth_identities` | planned |
| Remember me | 1.A Login | AUTH-0101 | `remember_me` field | `sessions.expires_at` | done |
| Login throttling | 1.A Login | AUTH-0307 | Error `RATE_LIMITED` | Redis or rate limit store | planned |
| Captcha after failures | 1.A Login | AUTH-0307 | `captcha_required` flag | Login attempt store | planned |
| Device tracking | 1.A Login | AUTH-0101 | Session response | `sessions`, `login_histories` | done |
| Manual register | 1.B Register | AUTH-0401 | `POST /auth/register` | `users`, `user_profiles` | planned |
| Register by admin | 1.B Register | USER-0203 | `POST /admin/users` | `users`, `user_profiles`, `auth_identities`, `user_roles`, `password_reset_tokens`, `audit_logs` | done |
| Invite user | 1.B Register | AUTH-0402 | `POST /auth/invite/accept` | `user_invitations` | planned |
| Email verification | 1.B Register | AUTH-0304 | Verify email endpoints | `email_verification_tokens` | planned |
| Phone verification | 1.B Register | AUTH-0305 | OTP endpoints | `otp_codes` | planned |
| Default role saat register | 1.B Register | AUTH-0401 | Register response | `roles`, `user_roles` | planned |
| Approval admin | 1.B Register | USER-0207 | Status endpoints | `users.status`, `audit_logs` | done |
| Access token | 1.C Token dan Session | AUTH-0002 | Auth scheme | None | done |
| Refresh token | 1.C Token dan Session | AUTH-0103 | `POST /auth/refresh-token` | `sessions`, `refresh_tokens` | done |
| Token rotation | 1.C Token dan Session | AUTH-0103 | Refresh response | `refresh_tokens.replaced_by_token_id` | done |
| Revoke token | 1.C Token dan Session | AUTH-0102, AUTH-0302 | Logout/session delete | `sessions.revoked_at` | done |
| Revoke all sessions | 1.C Token dan Session | AUTH-0303 | `POST /auth/logout-all` | `sessions.revoked_at` | planned |
| Session list | 1.C Token dan Session | AUTH-0301 | `GET /auth/sessions` | `sessions` | planned |
| Token blacklist | 1.C Token dan Session | AUTH-0102 | Token revoked errors | Redis or token store | planned |
| Forgot password | 1.D Forgot Password | AUTH-0105 | `POST /auth/forgot-password` | `password_reset_tokens` | done |
| Validate reset token | 1.D Forgot Password | AUTH-0106 | `POST /auth/reset-password/validate` | `password_reset_tokens` | done |
| Reset password | 1.D Forgot Password | AUTH-0107 | `POST /auth/reset-password` | `password_reset_tokens`, `users.password_hash` | done |
| Change password | 1.E Change Password | AUTH-0108 | `POST /auth/change-password` | `users.password_hash`, `sessions`, `refresh_tokens`, `audit_logs` | done |
| Password history | 1.E Change Password | AUTH-0108 | Change password validation | Future `password_histories` table | deferred |
| Password policy | 1.E Change Password | AUTH-0108 | Validation errors | Config | done |
| Enable 2FA | 1.F MFA | AUTH-0405 | `POST /auth/2fa/setup` | `user_two_factor_methods` | planned |
| Disable 2FA | 1.F MFA | AUTH-0405 | `POST /auth/2fa/disable` | `user_two_factor_methods` | planned |
| Verify 2FA login | 1.F MFA | AUTH-0405 | `POST /auth/2fa/verify` | `otp_codes`, 2FA tables | planned |
| Recovery codes | 1.F MFA | AUTH-0405 | `GET /auth/2fa/recovery-codes` | `user_recovery_codes` | planned |
| User domain model | 6 Database Design Saran | USER-0001 | Not public | `users`, `user_profiles`, `sessions`, `login_histories`, `audit_logs` | done |
| List user | 2.A User CRUD | USER-0201 | `GET /admin/users` | `users`, `user_roles`, `roles` | done |
| Detail user | 2.A User CRUD | USER-0202 | `GET /admin/users/:id` | `users`, `user_profiles`, `user_roles`, `roles`, `user_permissions`, `permissions`, `sessions`, `audit_logs` | done |
| Create user | 2.A User CRUD | USER-0203 | `POST /admin/users` | `users`, `user_profiles`, `auth_identities`, `user_roles`, `password_reset_tokens`, `audit_logs` | done |
| Update user | 2.A User CRUD | USER-0204 | `PATCH /admin/users/:id` | `users`, `user_profiles`, `auth_identities`, `audit_logs` | done |
| Delete user | 2.A User CRUD | USER-0205 | `DELETE /admin/users/:id` | `users.deleted_at`, `sessions.revoked_at`, `refresh_tokens.revoked_at`, `audit_logs` | done |
| Restore user | 2.A User CRUD | USER-0205 | `POST /admin/users/:id/restore` | `users.deleted_at`, `users.status`, `audit_logs` | done |
| Bulk delete | 2.A User CRUD | USER-0206 | `POST /admin/users/bulk-action` | `users.deleted_at`, `sessions`, `refresh_tokens`, `audit_logs` | done |
| Bulk update status | 2.A User CRUD | USER-0206 | `POST /admin/users/bulk-action` | `users.status`, `audit_logs` | done |
| User profile | 2.B User Profile | USER-0208 | Self profile endpoints | `user_profiles` | planned |
| Avatar | 2.B User Profile | USER-0208 | `PATCH /users/me/avatar` | `user_profiles.avatar_url` | planned |
| User status | 2.C User Status | USER-0207 | Status endpoints | `users.status`, `audit_logs` | done |
| Assign role | 2.D Role Management | USER-0209 | User role endpoints | `user_roles` | planned |
| Multiple roles | 2.D Role Management | USER-0209 | User role endpoints | `user_roles` | planned |
| Role priority | 2.D Role Management | USER-0211 | Role response | Future column if needed | deferred |
| Role scope | 2.D Role Management | USER-0209 | `organization_id` field | `user_roles.organization_id` | planned |
| List user permission | 2.E Permission Management | USER-0210 | User permission endpoints | `user_permissions` | planned |
| Effective permission | 2.E Permission Management | USER-0210 | `/auth/me`, user permissions | role permission tables | planned |
| Direct permission | 2.E Permission Management | USER-0210 | User permission endpoints | `user_permissions` | planned |
| Deny permission | 2.E Permission Management | USER-0210 | `effect=deny` | `user_permissions.effect` | planned |
| Permission cache | 2.E Permission Management | USER-0210 | Not public | Redis/cache | planned |
| Multi organization | 3 Organization | USER-0401 | Organization endpoints | `organizations` | planned |
| Switch organization | 3 Organization | USER-0401 | `POST /users/me/switch-organization` | session/context metadata | planned |
| Audit log user | 4 Audit Log User | USER-0301 | Audit endpoints | `audit_logs` | planned |
| Notification integration | 5 Notification Integration | AUTH-0105, AUTH-0107, AUTH-0108, AUTH-0304, AUTH-0402 | Not public | mail/whatsapp platform | in_progress |
| Public API documentation | 11 API Feature Lengkap | DOC-API-0001 | Full contract | None | planned |
| Security checklist | 12 Security Checklist | AUTH-0002, AUTH-0307, USER-0213 | Error and auth contract | token hash tables | planned |
| Middleware | 13 Middleware | USER-0213 | Protected endpoints | None | done |
| Seed super admin | User request | AUTH-0001, USER-0002 | Not public | Seed command and env | done |

## API to Task Index

| Endpoint | Task ID | Status |
| --- | --- | --- |
| `POST /auth/login` | AUTH-0101 | done |
| `POST /auth/logout` | AUTH-0102 | done |
| `POST /auth/logout-all` | AUTH-0303 | planned |
| `POST /auth/refresh-token` | AUTH-0103 | done |
| `GET /auth/me` | AUTH-0104 | done |
| `POST /auth/register` | AUTH-0401 | planned |
| `POST /auth/verify-email` | AUTH-0304 | planned |
| `POST /auth/resend-verification-email` | AUTH-0304 | planned |
| `POST /auth/forgot-password` | AUTH-0105 | done |
| `POST /auth/reset-password/validate` | AUTH-0106 | done |
| `POST /auth/reset-password` | AUTH-0107 | done |
| `POST /auth/change-password` | AUTH-0108 | done |
| `POST /auth/2fa/setup` | AUTH-0405 | planned |
| `POST /auth/2fa/verify` | AUTH-0405 | planned |
| `POST /auth/2fa/disable` | AUTH-0405 | planned |
| `GET /auth/2fa/recovery-codes` | AUTH-0405 | planned |
| `GET /auth/sessions` | AUTH-0301 | planned |
| `DELETE /auth/sessions/:id` | AUTH-0302 | planned |
| `GET /users/me` | USER-0208 | planned |
| `PATCH /users/me/profile` | USER-0208 | planned |
| `PATCH /users/me/avatar` | USER-0208 | planned |
| `GET /admin/users` | USER-0201 | done |
| `GET /admin/users/:id` | USER-0202 | done |
| `POST /admin/users` | USER-0203 | done |
| `PATCH /admin/users/:id` | USER-0204 | done |
| `DELETE /admin/users/:id` | USER-0205 | done |
| `POST /admin/users/:id/restore` | USER-0205 | done |
| `POST /admin/users/bulk-action` | USER-0206 | done |
| `PATCH /admin/users/:id/status` | USER-0207 | done |
| `POST /admin/users/:id/activate` | USER-0207 | done |
| `POST /admin/users/:id/suspend` | USER-0207 | done |
| `POST /admin/users/:id/ban` | USER-0207 | done |
| `GET /admin/users/:id/roles` | USER-0209 | planned |
| `POST /admin/users/:id/roles` | USER-0209 | planned |
| `DELETE /admin/users/:id/roles/:roleId` | USER-0209 | planned |
| `GET /admin/users/:id/permissions` | USER-0210 | planned |
| `POST /admin/users/:id/permissions` | USER-0210 | planned |
| `DELETE /admin/users/:id/permissions/:permissionId` | USER-0210 | planned |
| `GET /admin/users/:id/sessions` | AUTH-0301 | planned |
| `DELETE /admin/users/:id/sessions/:sessionId` | AUTH-0302 | planned |
| `GET /admin/users/:id/audit-logs` | USER-0301 | planned |
| `GET /admin/roles` | USER-0211 | planned |
| `GET /admin/roles/:id` | USER-0211 | planned |
| `POST /admin/roles` | USER-0211 | planned |
| `PATCH /admin/roles/:id` | USER-0211 | planned |
| `DELETE /admin/roles/:id` | USER-0211 | planned |
| `GET /admin/roles/:id/permissions` | USER-0211 | planned |
| `POST /admin/roles/:id/permissions` | USER-0211 | planned |
| `DELETE /admin/roles/:id/permissions/:permissionId` | USER-0211 | planned |
| `GET /admin/permissions` | USER-0212 | planned |
| `GET /admin/permissions/grouped` | USER-0212 | planned |
| `GET /admin/permission-matrix` | USER-0212 | planned |

## Migration to Task Index

| Migration | Related Task | Status |
| --- | --- | --- |
| `000002_create_user_auth_tables` | USER-0002, AUTH-0101, AUTH-0105, AUTH-0301, USER-0301 | done |
| `000003_normalize_permission_role_tables` | USER-0002, USER-0209, USER-0211, USER-0212, USER-0213 | done |
| `000004_create_user_permissions` | USER-0002, USER-0210 | done |
| `000005_create_invitations_and_2fa_tables` | AUTH-0402, AUTH-0405 | planned |
| `000006_create_organization_user_tables` | USER-0401 | planned |
| `000011_seed_password_changed_notification_template` | AUTH-0107 | done |

## Seed Traceability

| Seed Item | Env/Source | Related Task | Status |
| --- | --- | --- | --- |
| Super admin name | `SEED_ADMIN_NAME` | AUTH-0001 | done |
| Super admin username | `SEED_ADMIN_USERNAME` | AUTH-0001 | done |
| Super admin email | `SEED_ADMIN_EMAIL` | AUTH-0001 | done |
| Super admin password | `SEED_ADMIN_PASSWORD` | AUTH-0001 | done |
| Super admin phone | `SEED_ADMIN_WHATSAPP` | AUTH-0001 | done |
| Super admin role | `SEED_ADMIN_ROLE` | AUTH-0001 | done |
| Base user permissions | Static seed list | USER-0212 | done |
| Role permission assignment | Static seed list | USER-0211 | done |

## Decision Log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-06-11 | Auth helper ditempatkan di `internal/core/auth`, business rule user di `internal/modules/user/service` | Mengikuti arsitektur repo dan menghindari core auth bergantung langsung pada HTTP/module detail |
| 2026-06-11 | Seed super admin butuh env `SEED_ADMIN_USERNAME` dan `SEED_ADMIN_PASSWORD` tambahan | Request membutuhkan username dan password dari `.env`, tetapi `.env.example` belum menyediakan key tersebut |
| 2026-06-11 | Public API contract dibuat sebagai dokumen terpisah dari task development | Frontend dan aplikasi eksternal butuh referensi kontrak yang mudah dibaca tanpa membaca task internal |

## Development History

| Date | Event | Files | Notes |
| --- | --- | --- | --- |
| 2026-06-11 | Initial Auth/User planning documentation | `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-migration-seed-plan.md`, `docs/auth-user-traceability-index.md` | Breakdown dibuat dari `docs/reference-auth-user.md` untuk module `internal/core/auth` dan `internal/modules/user` |
| 2026-06-11 | Migration guide linked to Auth/User migration plan | `docs/migration-guide.md` | Menambahkan pointer ke rencana migration dan seed Auth/User |
| 2026-06-11 | Started `AUTH-0001` seed admin config | `.env.example`, `internal/config/config.go`, `internal/config/load.go` | Menambahkan env dan loader `SeedAdminConfig` sebagai langkah awal seed super admin |
| 2026-06-11 | Completed `AUTH-0001` auth config foundation | `.env.example`, `internal/config/config.go`, `internal/config/load.go`, `internal/config/validate.go`, `internal/app/app.go` | Menambahkan config TTL auth, password policy minimal, validasi secret production, dan validator seed admin |
| 2026-06-11 | Completed `AUTH-0002` auth helper foundation | `internal/core/auth` | Menambahkan password hasher, JWT HS256 manager, claims model, random token, token hash, dan unit test |
| 2026-06-11 | Completed `USER-0001` user domain model | `internal/modules/user/model` | Menambahkan model user, profile, role, permission, session, login history, audit log, status enum, permission effect, dan unit test |
| 2026-06-11 | Completed `USER-0002` database migration implementation | `migrations/000002_create_user_auth_tables.*.sql`, `migrations/000003_normalize_permission_role_tables.*.sql`, `migrations/000004_create_user_permissions.*.sql` | Menambahkan tabel user/auth inti, normalisasi role/permission kompatibel, direct user permission, dan verifikasi up/down |
| 2026-06-11 | Completed super admin seed command | `cmd/seed`, `internal/modules/user/seeder` | Menambahkan command `go run ./cmd/seed -name super-admin`, seed permission dasar, role system, user super admin, profile, identity lokal, dan role assignment |
| 2026-06-11 | Completed `AUTH-0101` email/username password login | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/app` | Menambahkan endpoint `POST /api/v1/auth/login`, password verification, status guard, access token, refresh token, session persistence, login history, remember me, dan unit test service |
| 2026-06-11 | Completed `AUTH-0102` logout | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository` | Menambahkan endpoint `POST /api/v1/auth/logout`, bearer token parsing, session revoke, logout/token revoked history, dan unit test service |
| 2026-06-11 | Completed `AUTH-0103` refresh token rotation | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository` | Menambahkan endpoint `POST /api/v1/auth/refresh-token`, refresh token table persistence, rotation atomik, old-token reuse detection, session revoke on reuse, dan unit test service |
| 2026-06-11 | Completed `AUTH-0104` current user | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/modules/user/dto` | Menambahkan endpoint `GET /api/v1/auth/me`, token/session validation, profile, roles, effective permissions, session metadata, dan unit test service |
| 2026-06-12 | Implemented auth middleware context for protected routes | `internal/core/middleware`, `internal/modules/user/service`, `internal/app`, `docs/auth-user-development-tasks.md`, `docs/auth-user-traceability-index.md` | Menambahkan Bearer auth middleware, active user/session check, user context untuk permission guard, dan protected route group |
| 2026-06-12 | Completed `USER-0213` RBAC middleware integration | `internal/core/middleware`, `docs/auth-user-development-tasks.md`, `docs/auth-user-traceability-index.md` | Menambahkan role middleware `RequireRole` dan `RequireAnyRole`, menjaga error forbidden konsisten, dan menandai middleware integration selesai |
| 2026-06-12 | Completed `AUTH-0105` forgot password | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/app`, `docs/auth-user-development-tasks.md`, `docs/auth-user-traceability-index.md` | Menambahkan endpoint `POST /api/v1/auth/forgot-password`, reset token hash + expiry, generic response anti user enumeration, dan publish event `auth.password_reset_requested` ke notification outbox |
| 2026-06-12 | Completed `AUTH-0106` validate reset token | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/modules/user/dto`, `docs/auth-user-development-tasks.md`, `docs/auth-user-traceability-index.md` | Menambahkan endpoint `POST /api/v1/auth/reset-password/validate`, validasi token hash, used token, expired token, dan response valid untuk halaman reset password |
| 2026-06-12 | Completed `AUTH-0107` reset password | `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/modules/user/dto`, `internal/core/notification`, `migrations/000011_seed_password_changed_notification_template.*.sql`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan endpoint `POST /api/v1/auth/reset-password`, password policy minimum length, konsumsi token dan revoke semua session secara atomik, integration test repository, serta event outbox `auth.password_changed` |
| 2026-06-12 | Completed `AUTH-0108` change password | `internal/app/router.go`, `internal/modules/user/handler`, `internal/modules/user/service`, `internal/modules/user/repository`, `internal/modules/user/dto`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `POST /api/v1/auth/change-password`, current password verification, minimum password policy, optional revoke device lain, audit log, notification outbox, unit test, dan integration test repository |
| 2026-06-12 | Completed `USER-0201` admin list users | `internal/app`, `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `GET /api/v1/admin/users`, permission guard, filter user, allowlist sorting, pagination metadata, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0202` admin detail user | `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `GET /api/v1/admin/users/:id`, identity/profile, role, direct permission, session summary, audit summary, soft-delete permission guard, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0203` admin create user | `internal/app/app.go`, `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `POST /api/v1/admin/users`, validasi identity dan role, transaction create user/profile/local identity/role/setup token/audit, one-time setup link melalui event `user.invited`, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0204` admin update user | `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `PATCH /api/v1/admin/users/:id`, partial identity/profile update, local identity sync, reset email/phone verification saat berubah, uniqueness handling, audit `user_updated`, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0205` soft delete and restore user | `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan protected endpoint `DELETE /api/v1/admin/users/:id` dan `POST /api/v1/admin/users/:id/restore`, revoke session/refresh token atomik, restore status dari audit metadata dengan fallback inactive, audit lifecycle, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0206` bulk user action | `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan endpoint `POST /api/v1/admin/users/bulk-action`, action delete/restore/update_status, permission dinamis, batas 100 item, structured partial failure, status audit, unit/handler test, dan integration test PostgreSQL |
| 2026-06-12 | Completed `USER-0207` user status management | `internal/modules/user/handler/user_handler.go`, `internal/modules/user/service/user_service.go`, `internal/modules/user/repository/user_repository.go`, `internal/modules/user/dto/user.go`, `api/openapi.yaml`, `docs/auth-user-development-tasks.md`, `docs/auth-user-public-api-contract.md`, `docs/auth-user-traceability-index.md` | Menambahkan generic dan shortcut status endpoints, permission `user.update_status`, reason validation untuk suspended/banned, shared status rule dengan bulk action, audit actor/old/new/reason, unit/handler test, dan integration test PostgreSQL |

## Update Rules

- Saat task mulai dikerjakan, ubah status menjadi `in_progress`.
- Saat endpoint selesai, update API contract dan OpenAPI.
- Saat migration dibuat, update migration index.
- Saat task selesai, ubah status menjadi `done` dan tambahkan catatan command/test di dokumen traceability atau development traceability jika sudah ada.
- Saat ada perubahan keputusan teknis, tambahkan entry baru di `Decision Log`.
- Saat ada milestone implementasi, tambahkan entry baru di `Development History`.
