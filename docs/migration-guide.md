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

## Rencana Migration Landing Page

Requirement data model, audit schema lama, dan urutan task migration Landing Page ada di:

```bash
docs/reference-landing-page.md
docs/landing-page-development-tasks.md
docs/landing-page-traceability-index.md
```

Sebelum membuat migration Landing Page, periksa table legacy `landing_pages` dan `brands` pada database aktual. Jika perlu normalisasi atau cleanup, buat migration terpisah dan jangan mengubah baseline atau migration lama.

## Rencana Migration Multi-Tenant

Architecture, urutan migration, platform organization, membership, domain, entitlement, audit, dan RLS ada di:

```bash
docs/reference-multi-tenant.md
docs/multi-tenant-development-tasks.md
docs/multi-tenant-traceability-index.md
docs/multi-tenant-schema-query-audit.md
```

Migration organization dan tenant context harus diselesaikan sebelum migration module baru mengandalkan `organization_id`. Backfill platform/customer ownership wajib dibuat eksplisit; jangan memakai `organization_id NULL` untuk mewakili platform organization.

## Rencana Migration Billing Plan

Konsep, task, dan traceability Plan/Billing/Subscription ada di:

```bash
docs/billing-plan-concept-reference.md
docs/billing-plan-development-tasks.md
docs/billing-plan-development-traceability.md
```

Dokumentasi billing ini lahir saat migration terakhir masih `000049`, dan implementasi billing saat ini sudah memakai `000050` sampai `000056` untuk plan catalog, subscriptions, invoices/payments, events, permission seed, feature/plan seed, dan plan entitlement seed.

Runtime entitlement dan usage counter sudah tersedia di:

```bash
migrations/000016_create_organization_entitlements.up.sql
```

Jangan membuat `billing_organization_entitlements` atau `billing_usage_counters` pada MVP tanpa rencana deprecation/migration yang eksplisit untuk menghindari dua source of truth.
