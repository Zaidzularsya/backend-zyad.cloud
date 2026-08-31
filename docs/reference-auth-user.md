Berikut breakdown **feature module User Management dan Auth** yang cocok untuk CRM / enterprise app kamu.

## 1. Modul Auth

Modul Auth fokus pada proses **identitas user**, login, logout, token, session, dan keamanan akses.

### A. Login

Fitur utama:

| Feature                             | Fungsi                                    |
| ----------------------------------- | ----------------------------------------- |
| Login email + password              | User masuk menggunakan email dan password |
| Login username + password           | Opsional jika sistem mendukung username   |
| Login WhatsApp OTP                  | User masuk via nomor WA dan OTP           |
| Login Google                        | Untuk social login                        |
| Login GitHub/GitLab                 | Cocok untuk internal developer/team       |
| Remember me                         | Session lebih panjang untuk user tertentu |
| Login throttling                    | Membatasi percobaan login gagal           |
| Captcha setelah gagal beberapa kali | Mencegah brute force                      |
| Device tracking                     | Mencatat device/browser/IP saat login     |

Endpoint contoh:

```txt
POST /auth/login
POST /auth/login/whatsapp/request-otp
POST /auth/login/whatsapp/verify-otp
POST /auth/google
POST /auth/github
POST /auth/logout
GET  /auth/me
```

---

### B. Register

Untuk CRM enterprise, register bisa dibuat fleksibel.

| Feature                    | Fungsi                                           |
| -------------------------- | ------------------------------------------------ |
| Register manual            | User daftar sendiri                              |
| Register by admin          | Admin membuat user dari dashboard                |
| Invite user                | Admin mengundang user lewat email/link           |
| Email verification         | Verifikasi email sebelum aktif                   |
| Phone verification         | Verifikasi nomor WhatsApp/HP                     |
| Default role saat register | Misalnya `member`, `editor`, atau `pending-user` |
| Approval admin             | User baru harus disetujui admin                  |

Endpoint contoh:

```txt
POST /auth/register
POST /auth/verify-email
POST /auth/resend-verification-email
POST /auth/invite/accept
```

---

### C. Token dan Session

Kalau backend kamu pakai API, biasanya pakai JWT atau session hybrid.

| Feature             | Fungsi                                            |
| ------------------- | ------------------------------------------------- |
| Access token        | Token pendek untuk akses API                      |
| Refresh token       | Token panjang untuk mendapatkan access token baru |
| Token rotation      | Refresh token diganti setiap digunakan            |
| Revoke token        | Logout dari satu device                           |
| Revoke all sessions | Logout dari semua device                          |
| Session list        | User bisa lihat device yang sedang login          |
| Token blacklist     | Mencegah token lama tetap digunakan               |

Endpoint contoh:

```txt
POST /auth/refresh-token
POST /auth/revoke-token
POST /auth/logout-all
GET  /auth/sessions
DELETE /auth/sessions/:id
```

---

### D. Forgot Password / Reset Password

Ini yang berhubungan dengan frontend URL dan backend URL yang pernah kamu bahas.

| Feature                | Fungsi                             |
| ---------------------- | ---------------------------------- |
| Request reset password | User minta link reset              |
| Generate reset token   | Backend membuat token sekali pakai |
| Kirim email/WA         | Backend kirim link frontend        |
| Validate reset token   | Cek token masih valid              |
| Reset password         | User mengubah password             |
| Expired token          | Token otomatis tidak berlaku       |
| Used token             | Token tidak bisa dipakai ulang     |

Endpoint contoh:

```txt
POST /auth/forgot-password
POST /auth/reset-password/validate
POST /auth/reset-password
```

Contoh link yang dikirim:

```txt
https://crm.domain.com/reset-password?token=xxxxx
```

Backend hanya generate token, sedangkan URL frontend diambil dari config:

```env
FRONTEND_URL=https://crm.domain.com
BACKEND_URL=https://api.domain.com
```

---

### E. Change Password

Untuk user yang sudah login.

| Feature                    | Fungsi                                     |
| -------------------------- | ------------------------------------------ |
| Change password            | User mengganti password                    |
| Require current password   | Harus input password lama                  |
| Force logout other devices | Setelah ganti password, device lain logout |
| Password history           | Tidak boleh pakai password lama            |
| Password policy            | Minimal karakter, angka, simbol, dsb       |

Endpoint contoh:

```txt
POST /auth/change-password
```

---

### F. Multi-Factor Authentication / 2FA

Untuk enterprise, ini penting.

| Feature            | Fungsi                            |
| ------------------ | --------------------------------- |
| Enable 2FA         | Mengaktifkan 2FA                  |
| Disable 2FA        | Menonaktifkan 2FA                 |
| TOTP Authenticator | Google Authenticator/Authy        |
| OTP WhatsApp/email | Kode dikirim lewat WA/email       |
| Recovery codes     | Kode cadangan                     |
| Verify 2FA login   | Verifikasi setelah login password |

Endpoint contoh:

```txt
POST /auth/2fa/setup
POST /auth/2fa/verify
POST /auth/2fa/disable
POST /auth/2fa/recovery-codes
```

---

## 2. Modul User Management

Modul ini fokus pada pengelolaan data user oleh admin/super admin.

### A. User CRUD

| Feature            | Fungsi                          |
| ------------------ | ------------------------------- |
| List user          | Menampilkan daftar user         |
| Detail user        | Melihat detail user             |
| Create user        | Admin membuat user              |
| Update user        | Admin mengubah data user        |
| Delete user        | Hapus user                      |
| Soft delete user   | User tidak dihapus permanen     |
| Restore user       | Mengembalikan user yang dihapus |
| Bulk delete        | Hapus banyak user               |
| Bulk update status | Aktif/nonaktif banyak user      |

Endpoint contoh:

```txt
GET    /admin/users
GET    /admin/users/:id
POST   /admin/users
PATCH  /admin/users/:id
DELETE /admin/users/:id
POST   /admin/users/:id/restore
POST   /admin/users/bulk-action
```

---

### B. User Profile

| Feature              | Fungsi                 |
| -------------------- | ---------------------- |
| Nama lengkap         | Identitas user         |
| Email                | Login dan notifikasi   |
| Username             | Opsional               |
| Phone/WhatsApp       | OTP dan notifikasi     |
| Avatar               | Foto profil            |
| Bio                  | Deskripsi pendek       |
| Job title            | Jabatan                |
| Department           | Divisi                 |
| Company/organization | Untuk multi organisasi |
| Address              | Opsional               |
| Timezone             | Untuk jadwal/log       |
| Language preference  | Bahasa UI              |

Endpoint contoh:

```txt
GET   /users/me
PATCH /users/me/profile
PATCH /users/me/avatar
```

---

### C. User Status

Status user sebaiknya jangan hanya aktif/nonaktif.

| Status      | Fungsi                           |
| ----------- | -------------------------------- |
| `active`    | User aktif                       |
| `inactive`  | User tidak aktif                 |
| `pending`   | Belum verifikasi/approval        |
| `suspended` | Diblokir sementara               |
| `banned`    | Diblokir permanen                |
| `deleted`   | Soft deleted                     |
| `invited`   | Sudah diundang tapi belum accept |

Feature tambahan:

```txt
PATCH /admin/users/:id/status
POST  /admin/users/:id/suspend
POST  /admin/users/:id/activate
POST  /admin/users/:id/ban
```

---

### D. Role Management

User bisa punya satu atau banyak role.

| Feature        | Fungsi                             |
| -------------- | ---------------------------------- |
| Assign role    | Menambahkan role ke user           |
| Remove role    | Menghapus role dari user           |
| Multiple roles | User bisa punya beberapa role      |
| Default role   | Role otomatis saat register        |
| Role priority  | Prioritas role jika konflik        |
| Role scope     | Role berdasarkan organisasi/cabang |

Endpoint contoh:

```txt
GET  /admin/users/:id/roles
POST /admin/users/:id/roles
DELETE /admin/users/:id/roles/:roleId
```

Contoh role:

```txt
super_admin
admin
content_manager
content_editor
reviewer
member
viewer
```

---

### E. Permission Management

Permission lebih granular dari role.

| Feature              | Fungsi                                             |
| -------------------- | -------------------------------------------------- |
| List permission user | Melihat permission user                            |
| Effective permission | Permission hasil gabungan semua role               |
| Direct permission    | Permission khusus langsung ke user                 |
| Deny permission      | Menolak permission tertentu meski role punya akses |
| Permission cache     | Cache permission untuk performa                    |

Contoh permission:

```txt
user.read
user.create
user.update
user.delete

cms.page.read
cms.page.create
cms.page.publish
cms.media.upload

role.read
role.assign
permission.manage
```

Endpoint contoh:

```txt
GET  /admin/users/:id/permissions
POST /admin/users/:id/permissions
DELETE /admin/users/:id/permissions/:permissionId
```

---

## 3. Organization / Tenant Support

Kalau CRM kamu nantinya bisa dipakai banyak client/perusahaan, ini penting.

| Feature                      | Fungsi                            |
| ---------------------------- | --------------------------------- |
| Multi organization           | Satu sistem banyak organisasi     |
| User belongs to organization | User terhubung ke organisasi      |
| Organization role            | Role berbeda per organisasi       |
| Switch organization          | User pindah konteks organisasi    |
| Isolated data access         | Data tidak bocor antar organisasi |
| Branch/department            | Struktur organisasi internal      |

Contoh struktur:

```txt
User A:
- Organization: ASPARMINAS
  Role: Admin

- Organization: PT Zyad Technovation Indonesia
  Role: Super Admin
```

Endpoint contoh:

```txt
GET  /users/me/organizations
POST /users/me/switch-organization
GET  /admin/organizations/:id/users
POST /admin/organizations/:id/users
```

---

## 4. Audit Log User

Untuk enterprise, setiap perubahan user wajib tercatat.

| Event              | Dicatat                     |
| ------------------ | --------------------------- |
| User login         | IP, device, waktu           |
| User logout        | Waktu logout                |
| Failed login       | Email/IP penyebab gagal     |
| Password changed   | Siapa yang mengubah         |
| Role assigned      | Admin yang assign           |
| Permission changed | Perubahan permission        |
| User suspended     | Alasan suspend              |
| Profile updated    | Field yang berubah          |
| Token revoked      | Device/session yang dicabut |

Endpoint contoh:

```txt
GET /admin/users/:id/audit-logs
GET /admin/audit-logs?module=user
```

---

## 5. Notification Integration

Auth dan user management sangat erat dengan notifikasi.

| Trigger               | Channel                     |
| --------------------- | --------------------------- |
| Register success      | Email/WhatsApp              |
| Verify email          | Email                       |
| Forgot password       | Email/WhatsApp              |
| Password changed      | Email/WhatsApp              |
| Login from new device | Email/WhatsApp              |
| Account suspended     | Email                       |
| Invitation user       | Email                       |
| Role changed          | Email/internal notification |

Sebaiknya backend punya service seperti:

```txt
NotificationService
EmailService
WhatsAppService
TemplateService
SignedUrlService
OtpService
```

---

## 6. Database Design Saran

Minimal table:

```txt
users
user_profiles
roles
permissions
role_permissions
user_roles
user_permissions
auth_identities
sessions
refresh_tokens
password_reset_tokens
email_verification_tokens
otp_codes
login_histories
audit_logs
organizations
organization_users
```

---

## 7. Struktur Table Inti

### users

```sql
users
- id
- name
- email
- username
- password_hash
- phone
- status
- email_verified_at
- phone_verified_at
- last_login_at
- created_at
- updated_at
- deleted_at
```

---

### auth_identities

Untuk social login.

```sql
auth_identities
- id
- user_id
- provider
- provider_user_id
- provider_email
- access_token
- refresh_token
- created_at
- updated_at
```

Contoh provider:

```txt
google
github
gitlab
facebook
whatsapp
local
```

---

### roles

```sql
roles
- id
- name
- slug
- description
- is_system
- created_at
- updated_at
```

---

### permissions

```sql
permissions
- id
- module
- action
- name
- slug
- description
- created_at
- updated_at
```

Contoh:

```txt
module: user
action: create
slug: user.create
```

---

### user_roles

```sql
user_roles
- id
- user_id
- role_id
- organization_id
- assigned_by
- assigned_at
```

---

### role_permissions

```sql
role_permissions
- id
- role_id
- permission_id
- created_at
```

---

### sessions

```sql
sessions
- id
- user_id
- refresh_token_hash
- device_name
- user_agent
- ip_address
- last_used_at
- expires_at
- revoked_at
- created_at
```

---

### password_reset_tokens

```sql
password_reset_tokens
- id
- user_id
- token_hash
- expires_at
- used_at
- created_at
```

Token sebaiknya disimpan dalam bentuk hash, bukan plain token.

---

### otp_codes

```sql
otp_codes
- id
- user_id
- purpose
- destination
- code_hash
- expires_at
- used_at
- attempts
- created_at
```

Purpose contoh:

```txt
login
verify_phone
reset_password
change_email
```

---

## 8. RBAC Permission Concept

Konsep akses:

```txt
User -> User Roles -> Roles -> Role Permissions -> Permissions
```

Contoh:

```txt
Role: Content Editor

Permissions:
- cms.page.read
- cms.page.create
- cms.page.update
- cms.media.upload
```

Role `Content Editor` tidak boleh:

```txt
- user.delete
- role.manage
- permission.manage
- system.setting.update
```

---

## 9. Menu Dashboard Admin

Struktur menu untuk User Management:

```txt
User Management
├── Users
│   ├── All Users
│   ├── Active Users
│   ├── Pending Users
│   ├── Suspended Users
│   └── Deleted Users
│
├── Roles
│   ├── All Roles
│   ├── Create Role
│   └── Assign Permissions
│
├── Permissions
│   ├── All Permissions
│   └── Permission Matrix
│
├── Invitations
│   ├── Pending Invitations
│   ├── Accepted Invitations
│   └── Expired Invitations
│
├── Sessions
│   ├── Active Sessions
│   └── Login History
│
└── Audit Logs
```

---

## 10. Frontend Pages

Untuk frontend CRM:

```txt
/auth/login
/auth/register
/auth/forgot-password
/auth/reset-password
/auth/verify-email
/auth/accept-invitation
/auth/2fa

/admin/users
/admin/users/create
/admin/users/:id
/admin/users/:id/edit
/admin/users/:id/roles
/admin/users/:id/permissions
/admin/users/:id/sessions
/admin/users/:id/audit-logs

/admin/roles
/admin/roles/create
/admin/roles/:id/edit
/admin/roles/:id/permissions

/admin/permissions
/admin/permission-matrix

/profile
/profile/security
/profile/sessions
```

---

## 11. API Feature Lengkap

### Auth API

```txt
POST /auth/login
POST /auth/logout
POST /auth/logout-all
POST /auth/refresh-token
GET  /auth/me

POST /auth/register
POST /auth/verify-email
POST /auth/resend-verification-email

POST /auth/forgot-password
POST /auth/reset-password/validate
POST /auth/reset-password

POST /auth/change-password

POST /auth/2fa/setup
POST /auth/2fa/verify
POST /auth/2fa/disable
GET  /auth/2fa/recovery-codes

GET  /auth/sessions
DELETE /auth/sessions/:id
```

---

### Admin User API

```txt
GET    /admin/users
GET    /admin/users/:id
POST   /admin/users
PATCH  /admin/users/:id
DELETE /admin/users/:id
POST   /admin/users/:id/restore

PATCH  /admin/users/:id/status
POST   /admin/users/:id/activate
POST   /admin/users/:id/suspend
POST   /admin/users/:id/ban

GET    /admin/users/:id/roles
POST   /admin/users/:id/roles
DELETE /admin/users/:id/roles/:roleId

GET    /admin/users/:id/permissions
POST   /admin/users/:id/permissions
DELETE /admin/users/:id/permissions/:permissionId

GET    /admin/users/:id/sessions
DELETE /admin/users/:id/sessions/:sessionId

GET    /admin/users/:id/audit-logs
```

---

### Role API

```txt
GET    /admin/roles
GET    /admin/roles/:id
POST   /admin/roles
PATCH  /admin/roles/:id
DELETE /admin/roles/:id

GET    /admin/roles/:id/permissions
POST   /admin/roles/:id/permissions
DELETE /admin/roles/:id/permissions/:permissionId
```

---

### Permission API

```txt
GET /admin/permissions
GET /admin/permissions/grouped
GET /admin/permission-matrix
```

---

## 12. Security Checklist

Minimal wajib ada:

```txt
Password di-hash pakai bcrypt/argon2
Refresh token disimpan sebagai hash
Reset token disimpan sebagai hash
OTP disimpan sebagai hash
Rate limit login
Rate limit forgot password
Rate limit OTP
Audit log untuk aksi penting
Session revoke
Email verification
Role-based access control
Permission-based middleware
Soft delete user
Tidak expose password_hash di response API
Tidak expose token plain di database
CORS hanya untuk frontend domain yang valid
Config FRONTEND_URL dan BACKEND_URL dipisah
```

---

## 13. Middleware Yang Dibutuhkan

```txt
AuthMiddleware
RoleMiddleware
PermissionMiddleware
VerifiedEmailMiddleware
ActiveUserMiddleware
TwoFactorMiddleware
RateLimitMiddleware
AuditLogMiddleware
OrganizationScopeMiddleware
```

Contoh flow request:

```txt
Request
 -> AuthMiddleware
 -> ActiveUserMiddleware
 -> PermissionMiddleware
 -> Controller
 -> Service
 -> Repository
 -> Response
```

---

## 14. Rekomendasi MVP

Untuk tahap awal, jangan langsung buat semua. Mulai dari ini dulu:

### Phase 1 — Auth Basic

```txt
Login email/password
Logout
Refresh token
Get current user /auth/me
Forgot password
Reset password
Change password
```

### Phase 2 — User Management

```txt
CRUD user
Status user
Assign role
List role
List permission
RBAC middleware
```

### Phase 3 — Security

```txt
Audit log
Login history
Session management
Revoke session
Rate limit
Email verification
```

### Phase 4 — Enterprise

```txt
Invitation user
2FA
Organization/tenant
Direct permission override
Permission matrix
Social login
WhatsApp OTP
```

---

## 15. Kesimpulan Desain

Untuk CRM enterprise kamu, struktur paling ideal adalah:

```txt
Auth
├── Login
├── Logout
├── Token
├── Session
├── Forgot Password
├── Reset Password
├── Email Verification
└── 2FA

User Management
├── User CRUD
├── Profile
├── Status
├── Role Assignment
├── Permission Assignment
├── Session Management
└── Audit Log

Access Control
├── Roles
├── Permissions
├── Role Permissions
├── User Roles
└── Middleware Guard
```

Saran aku: mulai dari **Auth Basic + User CRUD + Role Permission** dulu. Itu sudah cukup kuat untuk pondasi CRM. Setelah stabil, baru lanjut ke invitation, 2FA, audit log detail, dan multi-organization.
