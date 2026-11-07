CREATE INDEX IF NOT EXISTS idx_audit_logs_mikrotik_org_created
	ON audit_logs (organization_id, created_at DESC, id DESC)
	WHERE module = 'mikrotik';
