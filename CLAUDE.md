@AGENTS.md

## Claude Code — catatan tambahan

- Gunakan Plan mode untuk perubahan yang menyentuh migration baru, module `billing`/payment (DOKU), atau isolasi tenant/RLS — dampaknya lintas modul dan sulit di-rollback kalau salah.
- RLS PostgreSQL (`apply_organization_rls()`, migration 000019, `FORCE`) diterapkan ke tabel `landing_*`, `crm_*`, `receivable_*`, `mikrotik_*`, `catalog_*`, `asset_objects`, `mail_*`, `wa_*` (daftar lengkap: `docs/database-context.md` bagian 3). Tabel `organization_*`, `billing_*`, `product_*`, `subscription_*` tidak ber-RLS dan mengandalkan filter manual `organization_id`. Tetap sertakan filter `organization_id` eksplisit di semua query. Query `psql` manual ke tabel ber-RLS harus di dalam transaksi dengan `set_config('app.organization_id', '<uuid>', true)`, kalau tidak hasilnya 0 baris.
- Bersihkan file scratch/binary (script debug sekali pakai, hasil `go build`) sebelum commit — jangan biarkan masuk ke git.
- `go test ./...` mencakup unit test dan `*_integration_test.go`. Test integration butuh Postgres test DB terkonfigurasi lewat env `TEST_DB_*` (lihat README.md bagian "Integration Test Database") — jangan asumsikan otomatis jalan tanpa DB itu tersedia.
