# Permission Core Working Guide

Panduan ini berlaku untuk perubahan di `internal/core/permission`.

## Bahasa dan scope

- Jelaskan hasil kerja dalam Bahasa Indonesia.
- Gunakan Bahasa Inggris untuk code, command, file name, dan commit message.
- Jaga scope package ini sebagai shared core permission, bukan module bisnis CRM.
- Jangan menambah production dependency permission engine seperti Casbin tanpa konfirmasi.

## Tujuan package

`internal/core/permission` bertanggung jawab untuk:

- Menyediakan permission checker yang bisa dipakai handler dan middleware.
- Membaca role dan permission user dari database.
- Menyediakan middleware route-level permission.
- Menjaga kontrak permission lintas module.

Package ini tidak bertanggung jawab untuk:

- Auth login, JWT parsing, atau session management.
- Ownership rule detail milik module bisnis, seperti lead owner atau team hierarchy.
- Database access di luar tabel permission/role/user-role yang dibutuhkan checker.

## Struktur layer

- `handler`: endpoint administratif permission, hanya binding parameter dan response.
- `service`: business rule permission, error mapping, dan permission evaluation.
- `repository`: query database saja, tanpa business rule.
- `domain`: struct domain internal.
- `dto`: response/request shape untuk HTTP.
- `middleware`: adapter Gin untuk route protection.
- `policy`: helper evaluasi permission murni tanpa database.
- `seeder`: seed data permission dan role default.

## Database dan migration

- Semua perubahan tabel permission harus lewat migration di `migrations/`.
- Jangan memakai auto sync.
- Schema tetap `public`.
- Query repository saat ini memakai tabel dan kolom:
  - `permissions.id`
  - `permissions.permission_name`
  - `permissions.description`
  - `permissions.module_id`
  - `permissions.created_at`
  - `permissions.updated_at`
  - `roles.id`
  - `roles.role_name`
  - `roles.description`
  - `roles.created_at`
  - `roles.updated_at`
  - `role_permissions.role_id`
  - `role_permissions.permission_id`
  - `user_roles.user_id`
  - `user_roles.role_id`
- Jika mengubah nama kolom, update repository, DTO, migration, dan dokumentasi bersama-sama.

## Permission model

Model awal yang diimplementasikan adalah RBAC sederhana:

```txt
user -> role -> permission
```

Dokumentasi permission CRM menyebut scope/ABAC:

```txt
user -> role -> permission -> scope -> condition
```

Untuk saat ini, scope boleh disiapkan di database, tetapi evaluation di service masih memakai permission name saja. Jangan mengklaim ABAC sudah aktif sampai service dan repository benar-benar mengevaluasi scope/condition.

## Naming permission

- Gunakan format konsisten `resource.action` untuk permission baru, misalnya `lead.read`.
- Permission lama seperti `permission:read` boleh tetap ada sampai ada migration/seed cleanup eksplisit.
- Hindari membuat permission yang terlalu luas kecuali untuk admin platform.

## Error handling

- Service harus mengembalikan `internal/core/errors.AppError`.
- Repository boleh mengembalikan error database asli.
- Handler cukup memanggil `corehttp.Fail` untuk error.

## Testing

Minimal jalankan:

```bash
go test ./...
```

Untuk perubahan migration, validasi minimal:

```bash
go run ./cmd/migrate -direction up
```

Gunakan database development lokal, bukan production.
