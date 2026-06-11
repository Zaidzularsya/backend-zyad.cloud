# Migration Guide

Migration dipakai untuk semua perubahan database. Jangan memakai auto sync.

## Lokasi File

File migration berada di:

```bash
migrations/
```

Format nama file:

```bash
000001_create_permission_tables.up.sql
000001_create_permission_tables.down.sql
```

Setiap migration harus punya pasangan `up` dan `down`.

## Menjalankan Migration

Jalankan semua migration yang belum diterapkan:

```bash
go run ./cmd/migrate -direction up
```

Rollback satu migration terakhir:

```bash
go run ./cmd/migrate -direction down -steps 1
```

Gunakan direktori lain jika diperlukan:

```bash
go run ./cmd/migrate -direction up -dir migrations
```

## Tracking

Migration yang sudah berjalan dicatat di tabel:

```txt
schema_migrations
```

Tabel ini dibuat otomatis oleh command migration.

## Aturan

- Schema database memakai `public`.
- Perubahan schema harus dibuat sebagai migration baru.
- Cleanup schema harus dibuat sebagai migration terpisah agar traceable.
- Jangan mengubah migration lama setelah dipakai di environment bersama.

## Rencana Migration Auth/User

Rencana detail untuk tabel `users`, auth token/session, audit log, role/permission normalization, dan seed super admin ada di:

```bash
docs/auth-user-migration-seed-plan.md
```
