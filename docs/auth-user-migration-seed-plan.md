# Auth and User Migration and Seed Plan

Dokumen ini menjelaskan mekanisme migration database dan seed awal untuk kebutuhan Auth dan User.

## Prinsip Migration

- Semua perubahan database dibuat di direktori `migrations/`.
- Setiap migration punya file `.up.sql` dan `.down.sql`.
- Schema memakai `public`.
- Jangan mengubah migration lama yang sudah pernah dipakai.
- Cleanup schema dibuat sebagai migration terpisah agar traceable.
- Token sensitif disimpan dalam bentuk hash.

## Kondisi Saat Ini

Migration yang sudah ada:

- `migrations/000001_create_permission_tables.up.sql`
- `migrations/000001_create_permission_tables.down.sql`

Tabel yang sudah dibuat:

- `permissions`
- `roles`
- `role_permissions`
- `user_roles`

Catatan:

- `user_roles` sudah ada tetapi belum punya foreign key ke `users`, karena tabel `users` belum ada.
- `permissions` memakai kolom `permission_name`, sedangkan reference design mengusulkan `module`, `action`, `name`, dan `slug`.
- `roles` memakai kolom `role_name`, sedangkan reference design mengusulkan `name` dan `slug`.

Karena itu, implementasi Auth/User perlu migration tambahan dan mungkin migration cleanup kompatibel.

## Recommended Migration Sequence

### 000002_create_user_auth_tables

Tujuan:

- Membuat tabel inti user dan auth.

Tabel:

- `users`
- `user_profiles`
- `auth_identities`
- `sessions`
- `refresh_tokens`
- `password_reset_tokens`
- `email_verification_tokens`
- `otp_codes`
- `login_histories`
- `audit_logs`

Kolom minimum `users`:

```sql
id uuid primary key default gen_random_uuid()
name varchar(150) not null
email varchar(255) not null
username varchar(100)
password_hash text
phone varchar(50)
status varchar(30) not null default 'pending'
email_verified_at timestamp without time zone
phone_verified_at timestamp without time zone
last_login_at timestamp without time zone
created_at timestamp without time zone not null default now()
updated_at timestamp without time zone not null default now()
deleted_at timestamp without time zone
```

Constraint minimum:

- Unique active email.
- Unique active username jika tidak null.
- Check status in `active`, `inactive`, `pending`, `suspended`, `banned`, `deleted`, `invited`.

Catatan implementasi:

- Jika PostgreSQL partial index dipakai, unique email dan username sebaiknya mengabaikan `deleted_at`.
- `password_hash` nullable untuk social-only account atau invited user sebelum set password.

### 000003_normalize_permission_role_tables

Tujuan:

- Menyelaraskan tabel role dan permission dengan kebutuhan Auth/User.
- Menambahkan foreign key `user_roles.user_id -> users.id`.
- Menambahkan scope organization jika organization sudah tersedia.

Perubahan yang direkomendasikan:

- Tambah `roles.slug`.
- Tambah `roles.is_system`.
- Tambah unique `roles.slug`.
- Tambah `permissions.module`.
- Tambah `permissions.action`.
- Tambah `permissions.slug`.
- Tambah unique `permissions.slug`.
- Tambah `user_roles.organization_id` nullable.
- Tambah `user_roles.assigned_by` nullable.
- Tambah foreign key `user_roles.user_id`.

Catatan:

- Jika ingin menjaga kompatibilitas, jangan langsung drop `role_name` atau `permission_name`.
- Cleanup rename/drop kolom lama dilakukan di migration terpisah setelah code memakai kolom baru.

### 000004_create_user_permissions

Tujuan:

- Mendukung direct allow dan deny permission pada user.

Tabel:

- `user_permissions`

Kolom minimum:

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null
permission_id uuid not null
organization_id uuid
effect varchar(20) not null
assigned_by uuid
assigned_at timestamp without time zone not null default now()
created_at timestamp without time zone not null default now()
```

Constraint:

- `effect` hanya `allow` atau `deny`.
- Unique by `user_id`, `permission_id`, `organization_id`.

### 000005_create_invitations_and_2fa_tables

Tujuan:

- Mendukung invitation, 2FA, dan recovery codes.

Tabel:

- `user_invitations`
- `user_two_factor_methods`
- `user_recovery_codes`

Migration ini bisa ditunda sampai Phase 4.

### 000006_create_organization_user_tables

Tujuan:

- Mendukung multi organization dan switch organization.

Tabel:

- `organizations`
- `organization_users`

Migration ini bisa ditunda jika module organization akan dibangun terpisah.

## Table Detail

### user_profiles

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null unique
avatar_url text
bio text
job_title varchar(150)
department varchar(150)
company varchar(150)
address text
timezone varchar(100)
language varchar(20)
created_at timestamp without time zone not null default now()
updated_at timestamp without time zone not null default now()
```

### auth_identities

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null
provider varchar(50) not null
provider_user_id varchar(255) not null
provider_email varchar(255)
access_token_hash text
refresh_token_hash text
created_at timestamp without time zone not null default now()
updated_at timestamp without time zone not null default now()
```

Provider:

- `local`
- `google`
- `github`
- `gitlab`
- `facebook`
- `whatsapp`

### sessions

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null
refresh_token_hash text not null
device_name varchar(150)
user_agent text
ip_address inet
last_used_at timestamp without time zone
expires_at timestamp without time zone not null
revoked_at timestamp without time zone
created_at timestamp without time zone not null default now()
```

Catatan:

- Jika tabel `refresh_tokens` tetap dibuat terpisah, `sessions.refresh_token_hash` bisa dipindahkan ke `refresh_tokens`.
- Pilih salah satu desain saat implementasi agar tidak duplikasi tanggung jawab.

### refresh_tokens

```sql
id uuid primary key default gen_random_uuid()
session_id uuid not null
user_id uuid not null
token_hash text not null
expires_at timestamp without time zone not null
revoked_at timestamp without time zone
created_at timestamp without time zone not null default now()
replaced_by_token_id uuid
```

### password_reset_tokens

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null
token_hash text not null
expires_at timestamp without time zone not null
used_at timestamp without time zone
created_at timestamp without time zone not null default now()
```

### email_verification_tokens

```sql
id uuid primary key default gen_random_uuid()
user_id uuid not null
email varchar(255) not null
token_hash text not null
expires_at timestamp without time zone not null
used_at timestamp without time zone
created_at timestamp without time zone not null default now()
```

### otp_codes

```sql
id uuid primary key default gen_random_uuid()
user_id uuid
purpose varchar(50) not null
destination varchar(255) not null
code_hash text not null
expires_at timestamp without time zone not null
used_at timestamp without time zone
attempts integer not null default 0
created_at timestamp without time zone not null default now()
```

Purpose:

- `login`
- `verify_phone`
- `reset_password`
- `change_email`

### login_histories

```sql
id uuid primary key default gen_random_uuid()
user_id uuid
identifier varchar(255)
event varchar(50) not null
success boolean not null
ip_address inet
user_agent text
device_name varchar(150)
reason text
created_at timestamp without time zone not null default now()
```

### audit_logs

```sql
id uuid primary key default gen_random_uuid()
module varchar(100) not null
event varchar(100) not null
actor_user_id uuid
target_user_id uuid
target_type varchar(100)
target_id uuid
metadata jsonb
ip_address inet
user_agent text
created_at timestamp without time zone not null default now()
```

## Seed Super Admin

### Required Env

Seed super admin wajib membaca data dari `.env`:

```env
SEED_ADMIN_NAME=
SEED_ADMIN_USERNAME=
SEED_ADMIN_EMAIL=
SEED_ADMIN_PASSWORD=
SEED_ADMIN_WHATSAPP=
SEED_ADMIN_ROLE=super_admin
```

### Seed Rules

- Seed hanya berjalan melalui command eksplisit, bukan otomatis saat aplikasi start production.
- Password di-hash sebelum insert.
- Jika user dengan email sudah ada, seed harus idempotent.
- Jika role belum ada, buat role `super_admin`.
- Jika permission dasar belum ada, seed permission awal.
- Assign semua permission awal ke role `super_admin`.
- Assign role `super_admin` ke seed user.
- Set `status = active`.
- Set `email_verified_at = now()` untuk seed super admin.
- Jangan log password.

### Suggested Seed Command

Command yang direkomendasikan:

```bash
go run ./cmd/seed -name super-admin
```

Jika command seed belum ada, task implementasi harus membuat command baru di `cmd/seed` atau memakai command bootstrap yang sudah disepakati.

Status: command seed tersedia di `cmd/seed` dan dapat dijalankan dengan:

```bash
go run ./cmd/seed -name super-admin
```

### Base Permissions to Seed

Permission awal minimal:

```txt
user.read
user.create
user.update
user.delete
user.restore
user.update_status
user.session.read
user.session.revoke
role.read
role.create
role.update
role.delete
role.assign
permission.read
permission.manage
audit.read
organization.user.read
organization.user.manage
```

Permission CRM dari reference bisa ditambahkan saat module CRM terkait dibangun:

```txt
cms.page.read
cms.page.create
cms.page.publish
cms.media.upload
```

## Migration Verification

Command umum:

```bash
go run ./cmd/migrate -direction up
go run ./cmd/migrate -direction down -steps 1
```

Checklist:

- Migration up berhasil dari database kosong.
- Migration down berhasil minimal satu step.
- Foreign key valid.
- Unique index email dan username bekerja.
- Status constraint bekerja.
- Seed super admin bisa dijalankan ulang tanpa duplicate.

## Rollback Notes

- Rollback tabel user/auth akan menghapus data auth dan session.
- Jangan rollback migration production tanpa backup.
- Cleanup migration harus dipisah dari migration create table.
- Seed rollback tidak wajib menghapus super admin, kecuali ada command khusus untuk environment development.
