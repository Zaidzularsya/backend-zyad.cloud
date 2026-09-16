UPDATE organization_domains
SET
	ssl_status = 'pending',
	updated_at = now()
WHERE type = 'subdomain'
	AND ssl_status = 'not_required'
	AND deleted_at IS NULL;
