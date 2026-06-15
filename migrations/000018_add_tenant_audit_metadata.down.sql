DROP TRIGGER IF EXISTS trg_normalize_tenant_audit_context ON audit_logs;
DROP FUNCTION IF EXISTS normalize_tenant_audit_context();

DROP INDEX IF EXISTS idx_audit_logs_request_id;
DROP INDEX IF EXISTS idx_audit_logs_impersonation_session_id;
DROP INDEX IF EXISTS idx_audit_logs_effective_created_at;
DROP INDEX IF EXISTS idx_audit_logs_operator_created_at;
DROP INDEX IF EXISTS idx_audit_logs_membership_id;
DROP INDEX IF EXISTS idx_audit_logs_organization_created_at;

ALTER TABLE audit_logs
	DROP CONSTRAINT IF EXISTS fk_audit_logs_impersonation_session_id,
	DROP CONSTRAINT IF EXISTS fk_audit_logs_effective_user_id,
	DROP CONSTRAINT IF EXISTS fk_audit_logs_operator_user_id,
	DROP CONSTRAINT IF EXISTS fk_audit_logs_session_id,
	DROP CONSTRAINT IF EXISTS fk_audit_logs_membership_id,
	DROP CONSTRAINT IF EXISTS fk_audit_logs_organization_id,
	DROP CONSTRAINT IF EXISTS audit_logs_impersonation_context_check,
	DROP CONSTRAINT IF EXISTS audit_logs_resolution_source_check,
	DROP COLUMN IF EXISTS request_id,
	DROP COLUMN IF EXISTS resolution_source,
	DROP COLUMN IF EXISTS impersonation_session_id,
	DROP COLUMN IF EXISTS effective_user_id,
	DROP COLUMN IF EXISTS operator_user_id,
	DROP COLUMN IF EXISTS session_id,
	DROP COLUMN IF EXISTS membership_id,
	DROP COLUMN IF EXISTS organization_id;
