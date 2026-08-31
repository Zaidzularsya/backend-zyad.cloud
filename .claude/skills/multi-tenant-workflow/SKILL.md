---
description: Reading-order checklist untuk kerja di kapabilitas multi-tenant atau module manapun yang menyimpan data organization. Pakai kalau task menyentuh internal/modules/organization, tenant resolution, atau isolasi/schema/query organization_id.
---

# Multi-Tenant Workflow

Baca dokumen berikut secara berurutan sebelum mengerjakan capability multi-tenant atau module yang
menyimpan data organization:

1. `README.md` — konteks platform.
2. `docs/reference-multi-tenant.md` — architecture, ownership, isolation, dan platform organization.
3. `docs/multi-tenant-development-tasks.md` — breakdown pekerjaan.
4. `docs/multi-tenant-traceability-index.md` — requirement, API, migration, permission, dan dependency.
5. `docs/multi-tenant-schema-query-audit.md` — sebelum normalisasi schema/query yang sudah ada.
6. `docs/migration-guide.md` — sebelum membuat migration.

Zyad Cloud sendiri direpresentasikan sebagai platform organization. Landing Page marketing platform dan
Landing Page customer memakai module yang sama dengan `organization_id` yang konkret.

Lihat juga rule `.claude/rules/rls.md` — RLS PostgreSQL hanya aktif di tabel `landing_*`, modul lain wajib
filter manual `organization_id`.
