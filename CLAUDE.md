@AGENTS.md

## Claude Code — catatan tambahan

- Gunakan Plan mode untuk perubahan yang menyentuh migration baru, payment/invoice (`receivable` + DOKU),
  entitlement/kuota, atau isolasi tenant/RLS — dampaknya lintas modul dan sulit di-rollback kalau salah.
- Aturan RLS lengkap ada di `.claude/rules/rls.md` (dimuat otomatis). Ringkasnya: tabel `landing_*`, `crm_*`,
  `receivable_*`, `mikrotik_*`, `catalog_*`, `asset_objects`, `mail_*`, `wa_*` ber-RLS `FORCE`; tabel
  `organization_*`, `product_*`, user/auth, notification tidak. Tetap sertakan filter `organization_id`
  eksplisit di semua query. Query `psql` manual ke tabel ber-RLS harus di dalam transaksi dengan
  `set_config('app.organization_id', '<uuid>', true)`, kalau tidak hasilnya 0 baris.
- Skill reading-order per area ada di `.claude/skills/` (auth-user, notification, multi-tenant,
  landing-page, billing-plan). `billing-plan-workflow` sekarang berarti area produk/entitlement/receivable —
  modul `billing` sudah dihapus.
- Bersihkan file scratch/binary (script debug sekali pakai, hasil `go build`) sebelum commit — jangan biarkan
  masuk ke git. Hook `.claude/settings.json` juga memblokir `git commit` berisi pola tersebut.
- `go test ./...` hanya menjalankan unit test. File `*_integration_test.go` memakai build tag `integration`
  dan butuh Postgres test DB lewat env `TEST_DB_*` (lihat README.md bagian "Integration Test Database").
  Kalau DB test tidak tersedia, katakan integration test belum dijalankan — jangan klaim lulus.
- Jangan membaca atau menampilkan isi `.env`; redaksi secret yang muncul di output.
- Repo ini publik: catatan ops produksi (IP, host, runbook deploy, skrip cleansing) tidak boleh di-commit.
- Default target deploy pekerjaan adalah server DEV (skill `zyad-dev-deploy` di workspace induk); produksi
  hanya lewat skill `zyad-prod-deploy` dan hanya bila diminta eksplisit.
