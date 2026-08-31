---
description: Reading-order checklist untuk kerja di module Landing Page Builder. Pakai kalau task menyentuh internal/modules/landing atau kontrak API admin/public landing page yang dipakai Frontend.
---

# Landing Page Workflow

Baca dokumen berikut secara berurutan sebelum mengerjakan module Landing Page:

1. `README.md` — konteks platform.
2. `docs/reference-multi-tenant.md` — tenant context dan isolation contract.
3. `docs/reference-landing-page.md` — requirement, boundary, dan kapasitas.
4. `docs/landing-page-development-tasks.md` — breakdown pekerjaan.
5. `docs/landing-page-traceability-index.md` — traceability requirement, API, migration, permission, event, dan status.
6. `docs/landing-page-public-api-contract.md` — kontrak Frontend Vue dan public renderer.
7. `docs/migration-guide.md` — sebelum membuat migration.

Target canonical module adalah `internal/modules/landing` — modul paling matang dan satu-satunya yang
sudah dilindungi Row-Level Security PostgreSQL. Folder `internal/modules/landingpage/` kosong (sisa
scaffold lama) — jangan dipakai sebagai target module.
