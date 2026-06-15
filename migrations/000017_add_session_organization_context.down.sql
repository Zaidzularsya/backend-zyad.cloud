DROP TRIGGER IF EXISTS trg_validate_organization_impersonation_session
	ON organization_impersonation_sessions;
DROP FUNCTION IF EXISTS validate_organization_impersonation_session();

DROP TABLE IF EXISTS organization_impersonation_sessions;

DROP TRIGGER IF EXISTS trg_validate_session_organization_context ON sessions;
DROP FUNCTION IF EXISTS validate_session_organization_context();

DROP INDEX IF EXISTS idx_sessions_active_membership;
DROP INDEX IF EXISTS idx_sessions_active_organization;

ALTER TABLE sessions
	DROP CONSTRAINT IF EXISTS fk_sessions_active_membership_id,
	DROP CONSTRAINT IF EXISTS fk_sessions_active_organization_id,
	DROP CONSTRAINT IF EXISTS sessions_active_context_check,
	DROP COLUMN IF EXISTS active_membership_version,
	DROP COLUMN IF EXISTS active_membership_id,
	DROP COLUMN IF EXISTS active_organization_id;
