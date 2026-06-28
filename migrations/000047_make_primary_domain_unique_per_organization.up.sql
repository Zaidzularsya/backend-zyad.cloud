DROP INDEX IF EXISTS idx_organization_domains_primary_active_unique;

UPDATE organization_domains AS domain
SET is_primary = false,
	updated_at = now()
WHERE domain.is_primary = true
	AND domain.status = 'active'
	AND domain.deleted_at IS NULL
	AND EXISTS (
		SELECT 1
		FROM organization_domains newer
		WHERE newer.organization_id = domain.organization_id
			AND newer.is_primary = true
			AND newer.status = 'active'
			AND newer.deleted_at IS NULL
			AND (
				newer.created_at < domain.created_at
				OR (newer.created_at = domain.created_at AND newer.id < domain.id)
			)
	);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_primary_active_unique
	ON organization_domains(organization_id)
	WHERE is_primary = true
		AND status = 'active'
		AND deleted_at IS NULL;
