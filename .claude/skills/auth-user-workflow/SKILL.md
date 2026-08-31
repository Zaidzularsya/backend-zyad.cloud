---
description: Reading-order checklist untuk kerja di module Auth atau User (login, JWT, RBAC, session, password, Google OAuth). Pakai kalau task menyentuh internal/core/auth, internal/modules/user, atau kontrak API auth/user.
---

# Auth & User Workflow

Baca dokumen berikut secara berurutan sebelum mengerjakan module Auth/User:

1. `README.md` — konteks repo.
2. `docs/reference-auth-user.md` — source requirement.
3. `docs/auth-user-development-tasks.md` — breakdown pekerjaan.
4. `docs/auth-user-traceability-index.md` — kaitan requirement, API, migration, dan seed.
5. `docs/auth-user-public-api-contract.md` — kontrak response dan endpoint publik yang dipakai Frontend.
6. `docs/auth-user-migration-seed-plan.md` — urutan migration dan seed env.

Kalau task juga menyentuh Google OAuth, baca tambahan `docs/google-auth-development-tasks.md` dan
`docs/google-auth-traceability-index.md`.

Module stabil: `internal/core/auth` dan `internal/modules/user` — keduanya sudah dianggap production-ready,
jadi perubahan di sini harus backward-compatible kecuali user secara eksplisit minta breaking change.
