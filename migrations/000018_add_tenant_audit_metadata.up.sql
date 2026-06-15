ALTER TABLE audit_logs
	ADD COLUMN IF NOT EXISTS organization_id uuid,
	ADD COLUMN IF NOT EXISTS membership_id uuid,
	ADD COLUMN IF NOT EXISTS session_id uuid,
	ADD COLUMN IF NOT EXISTS operator_user_id uuid,
	ADD COLUMN IF NOT EXISTS effective_user_id uuid,
	ADD COLUMN IF NOT EXISTS impersonation_session_id uuid,
	ADD COLUMN IF NOT EXISTS resolution_source varchar(30),
	ADD COLUMN IF NOT EXISTS request_id varchar(100);

UPDATE audit_logs
SET
	operator_user_id = COALESCE(operator_user_id, actor_user_id),
	effective_user_id = COALESCE(effective_user_id, actor_user_id)
WHERE actor_user_id IS NOT NULL
	AND (operator_user_id IS NULL OR effective_user_id IS NULL);

ALTER TABLE audit_logs
	ADD CONSTRAINT audit_logs_resolution_source_check CHECK (
		resolution_source IS NULL
		OR resolution_source IN (
			'session',
			'header',
			'platform_host',
			'subdomain',
			'custom_domain',
			'worker',
			'internal'
		)
	),
	ADD CONSTRAINT audit_logs_impersonation_context_check CHECK (
		impersonation_session_id IS NULL
		OR (
			organization_id IS NOT NULL
			AND operator_user_id IS NOT NULL
			AND effective_user_id IS NOT NULL
		)
	),
	ADD CONSTRAINT fk_audit_logs_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE RESTRICT,
	ADD CONSTRAINT fk_audit_logs_membership_id
		FOREIGN KEY (membership_id) REFERENCES organization_memberships(id) ON DELETE SET NULL,
	ADD CONSTRAINT fk_audit_logs_session_id
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL,
	ADD CONSTRAINT fk_audit_logs_operator_user_id
		FOREIGN KEY (operator_user_id) REFERENCES users(id) ON DELETE SET NULL,
	ADD CONSTRAINT fk_audit_logs_effective_user_id
		FOREIGN KEY (effective_user_id) REFERENCES users(id) ON DELETE SET NULL,
	ADD CONSTRAINT fk_audit_logs_impersonation_session_id
		FOREIGN KEY (impersonation_session_id)
		REFERENCES organization_impersonation_sessions(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_audit_logs_organization_created_at
	ON audit_logs(organization_id, created_at DESC)
	WHERE organization_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_membership_id
	ON audit_logs(membership_id)
	WHERE membership_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_operator_created_at
	ON audit_logs(operator_user_id, created_at DESC)
	WHERE operator_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_effective_created_at
	ON audit_logs(effective_user_id, created_at DESC)
	WHERE effective_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_impersonation_session_id
	ON audit_logs(impersonation_session_id)
	WHERE impersonation_session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_request_id
	ON audit_logs(request_id)
	WHERE request_id IS NOT NULL;

CREATE OR REPLACE FUNCTION normalize_tenant_audit_context()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	impersonation_operator_user_id uuid;
	impersonation_target_organization_id uuid;
	impersonation_target_user_id uuid;
BEGIN
	IF NEW.impersonation_session_id IS NOT NULL THEN
		SELECT
			operator_user_id,
			target_organization_id,
			target_user_id
		INTO
			impersonation_operator_user_id,
			impersonation_target_organization_id,
			impersonation_target_user_id
		FROM organization_impersonation_sessions
		WHERE id = NEW.impersonation_session_id;

		IF NOT FOUND THEN
			RAISE EXCEPTION 'audit impersonation session does not exist';
		END IF;

		IF NEW.organization_id IS NOT NULL
			AND NEW.organization_id <> impersonation_target_organization_id THEN
			RAISE EXCEPTION 'audit organization does not match impersonation target';
		END IF;
		IF NEW.operator_user_id IS NOT NULL
			AND NEW.operator_user_id <> impersonation_operator_user_id THEN
			RAISE EXCEPTION 'audit operator does not match impersonation operator';
		END IF;
		IF impersonation_target_user_id IS NOT NULL
			AND NEW.effective_user_id IS NOT NULL
			AND NEW.effective_user_id <> impersonation_target_user_id THEN
			RAISE EXCEPTION 'audit effective user does not match impersonation target user';
		END IF;

		NEW.organization_id := impersonation_target_organization_id;
		NEW.operator_user_id := impersonation_operator_user_id;
		NEW.effective_user_id := COALESCE(
			impersonation_target_user_id,
			NEW.effective_user_id,
			NEW.actor_user_id,
			impersonation_operator_user_id
		);
	END IF;

	NEW.operator_user_id := COALESCE(NEW.operator_user_id, NEW.actor_user_id);
	NEW.effective_user_id := COALESCE(NEW.effective_user_id, NEW.actor_user_id);

	IF NEW.membership_id IS NOT NULL AND NOT EXISTS (
		SELECT 1
		FROM organization_memberships
		WHERE id = NEW.membership_id
			AND organization_id = NEW.organization_id
			AND user_id = NEW.effective_user_id
	) THEN
		RAISE EXCEPTION 'audit membership does not match organization and effective user';
	END IF;

	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_normalize_tenant_audit_context
	BEFORE INSERT ON audit_logs
	FOR EACH ROW
	EXECUTE FUNCTION normalize_tenant_audit_context();
