-- Host yang sudah dihapus (soft delete) tidak boleh memblokir pendaftaran ulang.
-- Ganti unique index global pada lower(canonical_host) dengan partial index
-- yang hanya berlaku untuk baris hidup (deleted_at IS NULL).
DROP INDEX IF EXISTS idx_organization_domains_canonical_host_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_canonical_host_unique
	ON organization_domains(lower(canonical_host))
	WHERE deleted_at IS NULL;
