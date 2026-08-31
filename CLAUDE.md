@AGENTS.md

## Claude Code — catatan tambahan

- Gunakan Plan mode untuk perubahan yang menyentuh migration baru, module `billing`/payment (DOKU), atau isolasi tenant/RLS — dampaknya lintas modul dan sulit di-rollback kalau salah.
- RLS PostgreSQL (`apply_organization_rls()`, migration 000019) hanya diterapkan ke tabel `landing_*`. Modul lain (`organization_*`, `billing_*`, `product_*`, `subscription_*`) mengandalkan filter manual `organization_id` di layer repository — selalu sertakan filter itu eksplisit saat menulis atau mengubah query di modul-modul tersebut. Detail: `docs/database-context.md`.
- Bersihkan file scratch/binary (script debug sekali pakai, hasil `go build`) sebelum commit — jangan biarkan masuk ke git.
- `go test ./...` mencakup unit test dan `*_integration_test.go`. Test integration butuh Postgres test DB terkonfigurasi lewat env `TEST_DB_*` (lihat README.md bagian "Integration Test Database") — jangan asumsikan otomatis jalan tanpa DB itu tersedia.
