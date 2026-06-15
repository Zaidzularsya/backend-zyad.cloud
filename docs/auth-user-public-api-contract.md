# Auth and User Public API Contract

Dokumen ini adalah kontrak konsumsi API publik untuk Frontend CRM dan aplikasi eksternal. Source of truth implementasi final tetap harus disinkronkan ke `api/openapi.yaml` ketika endpoint sudah dibangun.

## Prinsip Contract

- Base API memakai `APP_URL`.
- Frontend URL untuk link email memakai `APP_FRONTEND_URL`.
- Semua response JSON memakai envelope konsisten.
- Endpoint admin wajib memakai access token dan permission.
- Token dan password tidak pernah dikembalikan kecuali token login/refresh yang memang menjadi hasil auth.
- Breaking change harus dicatat di `docs/auth-user-traceability-index.md`.

## Auth Scheme

Access token dikirim via header:

```http
Authorization: Bearer <access_token>
```

Refresh token dikirim melalui request body atau secure http-only cookie sesuai keputusan implementasi. Jika memakai cookie, dokumentasikan nama cookie dan SameSite policy di OpenAPI.

## Standard Success Response

```json
{
  "success": true,
  "message": "Request processed successfully",
  "data": {}
}
```

## Standard Error Response

```json
{
  "success": false,
  "message": "Validation failed",
  "error": {
    "code": "VALIDATION_ERROR",
    "details": {
      "email": ["email is required"]
    }
  }
}
```

## Pagination Response

```json
{
  "success": true,
  "message": "Users retrieved successfully",
  "data": [],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

## Common Error Codes

| Code | HTTP | Meaning |
| --- | ---: | --- |
| `VALIDATION_ERROR` | 422 | Request body atau query tidak valid |
| `UNAUTHORIZED` | 401 | Access token tidak ada atau tidak valid |
| `FORBIDDEN` | 403 | User tidak punya permission |
| `NOT_FOUND` | 404 | Resource tidak ditemukan |
| `CONFLICT` | 409 | Email, username, role, atau permission duplicate |
| `RATE_LIMITED` | 429 | Terlalu banyak request |
| `AUTH_INVALID_CREDENTIALS` | 401 | Email/username/password salah |
| `AUTH_ACCOUNT_INACTIVE` | 403 | User tidak aktif |
| `AUTH_ACCOUNT_PENDING` | 403 | User belum approved atau belum verified |
| `AUTH_ACCOUNT_SUSPENDED` | 403 | User suspended |
| `AUTH_ACCOUNT_BANNED` | 403 | User banned |
| `AUTH_TOKEN_EXPIRED` | 401 | Token expired |
| `AUTH_TOKEN_REVOKED` | 401 | Token sudah dicabut |
| `AUTH_RESET_TOKEN_INVALID` | 422 | Reset token invalid |
| `AUTH_CURRENT_PASSWORD_INVALID` | 422 | Current password salah |
| `AUTH_PASSWORD_CONFIRMATION_MISMATCH` | 422 | Konfirmasi password tidak sama |
| `AUTH_PASSWORD_POLICY_FAILED` | 422 | Password tidak memenuhi policy |
| `AUTH_OTP_INVALID` | 422 | OTP invalid |

## Auth API

### POST /auth/login

Login memakai email atau username dan password.

Request:

```json
{
  "identifier": "admin@example.com",
  "password": "secret",
  "remember_me": true,
  "device_name": "Chrome on macOS"
}
```

Response:

```json
{
  "success": true,
  "message": "Login successful",
  "data": {
    "access_token": "jwt_access_token",
    "refresh_token": "refresh_token",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {
      "id": "uuid",
      "name": "Super Admin",
      "email": "admin@example.com",
      "username": "admin",
      "status": "active",
      "roles": ["super_admin"],
      "permissions": ["user.read", "user.create"]
    }
  }
}
```

### POST /auth/logout

Mencabut current session.

Auth: required.

Request:

```json
{
  "refresh_token": "refresh_token"
}
```

### POST /auth/logout-all

Mencabut semua session user login.

Auth: required.

Request:

```json
{
  "exclude_current": false
}
```

### POST /auth/refresh-token

Rotate refresh token dan membuat access token baru.

Request:

```json
{
  "refresh_token": "refresh_token"
}
```

Response:

```json
{
  "success": true,
  "message": "Token refreshed successfully",
  "data": {
    "access_token": "new_jwt_access_token",
    "refresh_token": "new_refresh_token",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

### GET /auth/me

Auth: required.

Response data minimal:

```json
{
  "id": "uuid",
  "name": "Super Admin",
  "email": "admin@example.com",
  "username": "admin",
  "phone": "+628123456789",
  "status": "active",
  "email_verified_at": "2026-01-01T00:00:00Z",
  "phone_verified_at": null,
  "profile": {
    "avatar_url": null,
    "bio": null,
    "job_title": "Administrator",
    "department": "IT",
    "timezone": "Asia/Jakarta",
    "language": "id"
  },
  "roles": ["super_admin"],
  "permissions": ["user.read", "user.create", "user.update", "user.delete"],
  "current_organization": null
}
```

### POST /auth/register

Self register. Phase enterprise jika tidak masuk MVP.

Request:

```json
{
  "name": "Jane Doe",
  "email": "jane@example.com",
  "username": "jane",
  "password": "secret",
  "phone": "+628123456789"
}
```

### POST /auth/verify-email

Request:

```json
{
  "token": "verification_token"
}
```

### POST /auth/resend-verification-email

Request:

```json
{
  "email": "jane@example.com"
}
```

### POST /auth/forgot-password

Request:

```json
{
  "email": "jane@example.com"
}
```

Response harus generic dan tidak membocorkan apakah email terdaftar.

### POST /auth/reset-password/validate

Request:

```json
{
  "token": "reset_token"
}
```

### POST /auth/reset-password

Request:

```json
{
  "token": "reset_token",
  "password": "new_secret",
  "password_confirmation": "new_secret"
}
```

Efek sukses:

- Password disimpan sebagai hash baru.
- Reset token ditandai sudah digunakan dan tidak dapat dipakai ulang.
- Semua session dan refresh token user dicabut.
- Event notifikasi `auth.password_changed` dimasukkan ke outbox secara best effort.

Error khusus:

- `AUTH_RESET_TOKEN_INVALID` jika token tidak ditemukan, kedaluwarsa, atau sudah digunakan.
- `AUTH_PASSWORD_CONFIRMATION_MISMATCH` jika konfirmasi password berbeda.
- `AUTH_PASSWORD_POLICY_FAILED` jika password tidak memenuhi minimum length dari config.

### POST /auth/change-password

Auth: required.

Request:

```json
{
  "current_password": "old_secret",
  "new_password": "new_secret",
  "new_password_confirmation": "new_secret",
  "logout_other_devices": true
}
```

Efek sukses:

- Password disimpan sebagai hash baru.
- Current session tetap aktif.
- Bila `logout_other_devices=true`, seluruh session dan refresh token selain current session dicabut.
- Audit event `password_changed` dicatat tanpa menyimpan password.
- Event notifikasi `auth.password_changed` dimasukkan ke outbox secara best effort.

Error khusus:

- `AUTH_CURRENT_PASSWORD_INVALID` jika current password salah.
- `AUTH_PASSWORD_CONFIRMATION_MISMATCH` jika konfirmasi password baru berbeda.
- `AUTH_PASSWORD_POLICY_FAILED` jika password baru tidak memenuhi minimum length dari config.

### GET /auth/sessions

Auth: required.

Response item:

```json
{
  "id": "uuid",
  "device_name": "Chrome on macOS",
  "ip_address": "127.0.0.1",
  "user_agent": "Mozilla/5.0",
  "last_used_at": "2026-01-01T00:00:00Z",
  "expires_at": "2026-01-08T00:00:00Z",
  "is_current": true
}
```

### DELETE /auth/sessions/:id

Auth: required.

Mencabut session milik user login.

### POST /auth/login/whatsapp/request-otp

Request:

```json
{
  "phone": "+628123456789"
}
```

### POST /auth/login/whatsapp/verify-otp

Request:

```json
{
  "phone": "+628123456789",
  "otp": "123456",
  "device_name": "Chrome on macOS"
}
```

### POST /auth/2fa/setup

Auth: required.

### POST /auth/2fa/verify

Auth: required for setup flow, partial auth for login challenge.

### POST /auth/2fa/disable

Auth: required.

### GET /auth/2fa/recovery-codes

Auth: required.

## Self User API

### GET /users/me

Auth: required.

Return data sama dengan `GET /auth/me`, namun boleh lebih detail untuk halaman profile.

### PATCH /users/me/profile

Auth: required.

Request:

```json
{
  "name": "Jane Doe",
  "phone": "+628123456789",
  "bio": "Short bio",
  "job_title": "Editor",
  "department": "Content",
  "address": "Jakarta",
  "timezone": "Asia/Jakarta",
  "language": "id"
}
```

### PATCH /users/me/avatar

Auth: required.

Request content type:

```txt
multipart/form-data
```

## Admin User API

Semua endpoint admin membutuhkan permission terkait.

### GET /admin/users

Permission: `user.read`

Query:

| Name | Type | Required | Notes |
| --- | --- | --- | --- |
| `page` | integer | no | Default 1 |
| `per_page` | integer | no | Default 20 |
| `search` | string | no | Search name, email, username |
| `status` | string | no | active, inactive, pending, suspended, banned, deleted, invited |
| `role` | string | no | Role slug |
| `organization_id` | uuid | no | Untuk multi organization |
| `include_deleted` | boolean | no | Butuh permission khusus |
| `created_from` | date | no | Format `YYYY-MM-DD`, inclusive |
| `created_to` | date | no | Format `YYYY-MM-DD`, inclusive |
| `sort` | string | no | Example `created_at` |
| `direction` | string | no | `asc` atau `desc` |

Allowed `sort`:

- `name`
- `email`
- `status`
- `created_at`
- `updated_at`

`include_deleted=true` membutuhkan permission tambahan `user.restore`.

Response:

```json
{
  "success": true,
  "message": "users retrieved successfully",
  "data": [
    {
      "id": "uuid",
      "name": "Jane Doe",
      "email": "jane@example.com",
      "username": "jane",
      "phone": "+628123456789",
      "status": "active",
      "email_verified_at": "2026-06-01T00:00:00Z",
      "phone_verified_at": null,
      "last_login_at": "2026-06-12T08:00:00Z",
      "roles": ["admin"],
      "created_at": "2026-06-01T00:00:00Z",
      "updated_at": "2026-06-12T08:00:00Z",
      "deleted_at": null
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 1,
    "total_pages": 1
  }
}
```

Response tidak mengandung `password_hash`.

### GET /admin/users/:id

Permission: `user.read`

Query:

| Name | Type | Required | Notes |
| --- | --- | --- | --- |
| `include_deleted` | boolean | no | Default `false`; jika `true` membutuhkan `user.restore` |

Response data:

```json
{
  "id": "uuid",
  "name": "Jane Doe",
  "email": "jane@example.com",
  "username": "jane",
  "phone": "+628123456789",
  "status": "active",
  "email_verified_at": "2026-06-01T00:00:00Z",
  "phone_verified_at": null,
  "last_login_at": "2026-06-12T08:00:00Z",
  "profile": {
    "avatar_url": "",
    "bio": "Administrator account",
    "job_title": "Administrator",
    "department": "IT",
    "company": "Zyad Cloud",
    "address": "Jakarta",
    "timezone": "Asia/Jakarta",
    "language": "id"
  },
  "roles": [
    {
      "id": "uuid",
      "name": "Admin",
      "slug": "admin",
      "organization_id": "",
      "assigned_at": "2026-06-01T00:00:00Z"
    }
  ],
  "direct_permissions": [
    {
      "id": "uuid",
      "permission_id": "uuid",
      "slug": "user.read",
      "effect": "allow",
      "organization_id": "",
      "assigned_at": "2026-06-01T00:00:00Z"
    }
  ],
  "sessions_summary": {
    "total": 3,
    "active": 1,
    "revoked": 1,
    "expired": 1,
    "last_active_at": "2026-06-12T08:00:00Z"
  },
  "audit_summary": {
    "total": 10,
    "last_event": "password_changed",
    "last_event_at": "2026-06-12T08:00:00Z",
    "last_actor_id": "uuid",
    "last_ip_address": "127.0.0.1"
  },
  "created_at": "2026-06-01T00:00:00Z",
  "updated_at": "2026-06-12T08:00:00Z",
  "deleted_at": null
}
```

`direct_permissions` hanya berisi assignment langsung pada user. Effective permission dari role tetap dihitung melalui permission service.

Error:

- `USER_NOT_FOUND` dengan HTTP `404` jika user tidak ditemukan atau soft-deleted tanpa akses `include_deleted`.

### POST /admin/users

Permission: `user.create`

Request:

```json
{
  "name": "Jane Doe",
  "email": "jane@example.com",
  "username": "jane",
  "phone": "+628123456789",
  "password": "temporary_secret",
  "status": "active",
  "roles": [
    {
      "role_id": "uuid",
      "organization_id": null
    }
  ],
  "send_invitation": false
}
```

Response `201` memakai struktur detail user yang sama dengan `GET /admin/users/:id`.

Notes:

- Jika `password` tidak dikirim, status default menjadi `invited` dan sistem mengirim one-time password setup link.
- Jika `send_invitation=true`, setup link juga dibuat saat temporary password disediakan.
- Error `USER_EMAIL_ALREADY_EXISTS` atau `USER_USERNAME_ALREADY_EXISTS` memakai HTTP `409`.
- Error `USER_ROLE_NOT_FOUND` memakai HTTP `422`.

### PATCH /admin/users/:id

Permission: `user.update`

Request accepts partial fields:

```json
{
  "name": "Jane Updated",
  "email": "jane.updated@example.com",
  "username": "jane.updated",
  "phone": "+628987654321",
  "job_title": "Senior Editor",
  "department": "Content"
}
```

Semua field bersifat optional, tetapi minimal satu field harus dikirim. Field profile dapat dikosongkan dengan mengirim string kosong.

Notes:

- Perubahan email mereset `email_verified_at`.
- Perubahan phone mereset `phone_verified_at`.
- `status` tidak dapat diubah melalui endpoint ini; gunakan endpoint status yang membutuhkan permission `user.update_status`.
- Error uniqueness menggunakan `USER_EMAIL_ALREADY_EXISTS` atau `USER_USERNAME_ALREADY_EXISTS` dengan HTTP `409`.

### DELETE /admin/users/:id

Permission: `user.delete`

Soft delete user.

Delete mengubah status menjadi `deleted`, mengisi `deleted_at`, serta mencabut seluruh session dan refresh token user.

### POST /admin/users/:id/restore

Permission: `user.restore`

Response `200` memakai struktur detail user. Status sebelum delete dipulihkan dari audit metadata; fallback untuk data lama adalah `inactive`. Session lama tetap revoked.

Error `USER_EMAIL_ALREADY_EXISTS` atau `USER_USERNAME_ALREADY_EXISTS` memakai HTTP `409` jika identity user sudah dipakai user aktif lain saat restore.

### POST /admin/users/bulk-action

Permission depends on action.

Request:

```json
{
  "action": "update_status",
  "user_ids": ["uuid"],
  "payload": {
    "status": "inactive",
    "reason": "Temporary deactivation"
  }
}
```

Allowed action:

- `delete`
- `restore`
- `update_status`

Maksimal `100` user ID per request. Permission mengikuti action:

- `delete`: `user.delete`
- `restore`: `user.restore`
- `update_status`: `user.update_status`

Status `suspended` dan `banned` membutuhkan `payload.reason`.

Response `200` tetap digunakan saat sebagian item gagal:

```json
{
  "action": "delete",
  "total": 2,
  "succeeded": 1,
  "failed": 1,
  "results": [
    {
      "user_id": "uuid-1",
      "success": true
    },
    {
      "user_id": "uuid-2",
      "success": false,
      "error": {
        "code": "USER_NOT_FOUND",
        "message": "user not found"
      }
    }
  ]
}
```

### PATCH /admin/users/:id/status

Permission: `user.update_status`

Request:

```json
{
  "status": "suspended",
  "reason": "Policy violation"
}
```

### POST /admin/users/:id/activate

Permission: `user.update_status`

### POST /admin/users/:id/suspend

Permission: `user.update_status`

Request:

```json
{
  "reason": "Security review"
}
```

### POST /admin/users/:id/ban

Permission: `user.update_status`

Request:

```json
{
  "reason": "Permanent policy violation"
}
```

Semua endpoint status membutuhkan permission `user.update_status` dan mengembalikan detail user terbaru.

Rules:

- Generic endpoint menerima `active`, `inactive`, `pending`, `suspended`, `banned`, dan `invited`.
- Status `deleted` hanya dapat diterapkan melalui endpoint soft delete.
- `suspended` dan `banned` membutuhkan `reason`.
- User dengan status selain `active` ditolak saat login, refresh token, dan access-token authentication.
- Setiap perubahan mencatat actor, status lama, status baru, dan reason pada audit log.

### GET /admin/users/:id/roles

Permission: `role.read`

### POST /admin/users/:id/roles

Permission: `role.assign`

Request:

```json
{
  "role_id": "uuid",
  "organization_id": null
}
```

### DELETE /admin/users/:id/roles/:roleId

Permission: `role.assign`

### GET /admin/users/:id/permissions

Permission: `permission.read`

### POST /admin/users/:id/permissions

Permission: `permission.manage`

Request:

```json
{
  "permission_id": "uuid",
  "effect": "allow",
  "organization_id": null
}
```

Allowed effect:

- `allow`
- `deny`

### DELETE /admin/users/:id/permissions/:permissionId

Permission: `permission.manage`

### GET /admin/users/:id/sessions

Permission: `user.session.read`

### DELETE /admin/users/:id/sessions/:sessionId

Permission: `user.session.revoke`

### GET /admin/users/:id/audit-logs

Permission: `audit.read`

## Role API

### GET /admin/roles

Permission: `role.read`

### GET /admin/roles/:id

Permission: `role.read`

### POST /admin/roles

Permission: `role.create`

Request:

```json
{
  "name": "Content Editor",
  "slug": "content_editor",
  "description": "Can manage content draft",
  "is_system": false
}
```

### PATCH /admin/roles/:id

Permission: `role.update`

### DELETE /admin/roles/:id

Permission: `role.delete`

System role tidak boleh dihapus.

### GET /admin/roles/:id/permissions

Permission: `role.read`

### POST /admin/roles/:id/permissions

Permission: `permission.manage`

Request:

```json
{
  "permission_id": "uuid",
  "scope": "organization"
}
```

Allowed scope:

- `none`
- `own`
- `team`
- `branch`
- `department`
- `organization`
- `all`

### DELETE /admin/roles/:id/permissions/:permissionId

Permission: `permission.manage`

## Permission API

### GET /admin/permissions

Permission: `permission.read`

### GET /admin/permissions/grouped

Permission: `permission.read`

### GET /admin/permission-matrix

Permission: `permission.read`

## Organization API for Auth Context

### GET /users/me/organizations

Auth: required.

Response contains active organization memberships and an `is_current` marker derived from the authenticated session snapshot.

### POST /users/me/switch-organization

Auth: required.

Request:

```json
{
  "organization_id": "uuid"
}
```

The selected organization must be active and have an active membership for the authenticated user. The switch updates only the current session and is effective on the next request. Tokens are not rotated because organization identity is not stored in the current JWT claims.

### GET /admin/organizations/:id/users

Permission: `organization.user.read`

### POST /admin/organizations/:id/users

Permission: `organization.user.manage`

## API Documentation Deliverables

Wajib dibuat saat implementasi:

- Update `api/openapi.yaml` untuk endpoint yang sudah aktif.
- Tambahkan example request dan response.
- Tandai endpoint Phase 4 sebagai planned jika belum aktif.
- Dokumentasikan permission tiap endpoint.
- Dokumentasikan rate limit endpoint auth.
