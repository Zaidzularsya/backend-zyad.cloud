-- Kembalikan unique index global. Bisa gagal jika ada host duplikat pada baris
-- yang sudah dihapus; hapus duplikatnya secara manual sebelum rollback.
DROP INDEX IF EXISTS idx_organization_domains_canonical_host_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_canonical_host_unique
	ON organization_domains(lower(canonical_host));
