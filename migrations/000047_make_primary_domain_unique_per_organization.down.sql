DROP INDEX IF EXISTS idx_organization_domains_primary_active_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_primary_active_unique
	ON organization_domains(organization_id, type)
	WHERE is_primary = true
		AND status = 'active'
		AND deleted_at IS NULL;
