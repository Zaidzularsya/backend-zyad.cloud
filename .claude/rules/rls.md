---
paths:
  - "internal/modules/**/repository/**/*.go"
  - "internal/modules/**/service/**/*.go"
---

# Isolasi Tenant (RLS)

Row-Level Security PostgreSQL (`apply_organization_rls()`, migration 000019) hanya diterapkan ke tabel
`landing_*`. Untuk tabel lain (`organization_*`, `billing_*`, `product_*`, `subscription_*`) **tidak ada**
guard di level database — isolasi tenant murni bergantung pada filter manual di repository.

Kalau kamu menulis atau mengubah query di modul selain `landing`:

- Selalu sertakan filter `organization_id` secara eksplisit di setiap SELECT/UPDATE/DELETE yang menyentuh
  data tenant.
- Jangan asumsikan context/middleware otomatis menambahkan filter tenant di level database — itu hanya
  berlaku untuk tabel `landing_*`.
- Kalau ragu apakah tabel yang kamu sentuh sudah punya RLS, cek `docs/database-context.md` atau
  `docs/multi-tenant-rls.md`.

Modul `landing` sudah terlindungi RLS — filter manual di sana bersifat defense-in-depth, bukan satu-satunya
lapisan proteksi.
