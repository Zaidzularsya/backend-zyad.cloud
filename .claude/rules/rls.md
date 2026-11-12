# Isolasi Tenant (RLS)

Row-Level Security PostgreSQL (`apply_organization_rls()`, migration 000019) diterapkan dengan
`FORCE ROW LEVEL SECURITY` ke banyak tabel tenant, bukan hanya `landing_*`: juga `crm_*`, `receivable_*`,
`mikrotik_*`, `catalog_*`, `asset_objects`, `mail_*`/`user_mailboxes`, dan `wa_*`. Daftar lengkap dan query
untuk mengeceknya ada di `docs/database-context.md` bagian 3.

Tabel **tanpa** RLS: `organization_*`, `billing_*`, `product_*`, `customer_subscriptions`,
`subscription_events`, user/auth, notification. Di sana isolasi tenant murni bergantung pada filter manual.

Aturan saat menulis atau mengubah query:

- Selalu sertakan filter `organization_id` eksplisit di setiap SELECT/UPDATE/DELETE data tenant, termasuk di
  tabel ber-RLS (RLS adalah defense-in-depth, bukan satu-satunya lapisan).
- Repository tabel ber-RLS harus berjalan lewat `TenantTransactor`/`withScopedTx` yang memanggil
  `set_config('app.organization_id', …, true)`; tanpa itu query mengembalikan 0 baris tanpa error.
- Query manual di `psql` ke tabel ber-RLS: jalankan di dalam transaksi dengan
  `SELECT set_config('app.organization_id', '<uuid>', true);` dan `SET LOCAL TimeZone = 'UTC';`.
- Tabel tenant baru wajib memanggil `apply_organization_rls('<table>'::regclass)` di migration-nya
  (lihat `docs/multi-tenant-rls.md`).
