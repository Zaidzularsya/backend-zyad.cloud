ALTER TABLE sessions
	ADD COLUMN IF NOT EXISTS active_organization_id uuid,
	ADD COLUMN IF NOT EXISTS active_membership_id uuid,
	ADD COLUMN IF NOT EXISTS active_membership_version bigint;

ALTER TABLE sessions
	ADD CONSTRAINT sessions_active_context_check CHECK (
		(
			active_organization_id IS NULL
			AND active_membership_id IS NULL
			AND active_membership_version IS NULL
		)
		OR (
			active_organization_id IS NOT NULL
			AND active_membership_id IS NOT NULL
			AND active_membership_version IS NOT NULL
			AND active_membership_version > 0
		)
	),
	ADD CONSTRAINT fk_sessions_active_organization_id
		FOREIGN KEY (active_organization_id) REFERENCES organizations(id) ON DELETE RESTRICT,
	ADD CONSTRAINT fk_sessions_active_membership_id
		FOREIGN KEY (active_membership_id) REFERENCES organization_memberships(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_sessions_active_organization
	ON sessions(active_organization_id)
	WHERE active_organization_id IS NOT NULL AND revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_active_membership
	ON sessions(active_membership_id, active_membership_version)
	WHERE active_membership_id IS NOT NULL AND revoked_at IS NULL;

CREATE OR REPLACE FUNCTION validate_session_organization_context()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	membership_version bigint;
BEGIN
	IF NEW.active_organization_id IS NULL THEN
		NEW.active_membership_id := NULL;
		NEW.active_membership_version := NULL;
		RETURN NEW;
	END IF;

	SELECT version
	INTO membership_version
	FROM organization_memberships
	WHERE id = NEW.active_membership_id
		AND organization_id = NEW.active_organization_id
		AND user_id = NEW.user_id
		AND status = 'active'
		AND removed_at IS NULL;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'active organization requires an active membership owned by the session user';
	END IF;

	NEW.active_membership_version := membership_version;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_validate_session_organization_context
	BEFORE INSERT OR UPDATE OF
		user_id,
		active_organization_id,
		active_membership_id,
		active_membership_version
	ON sessions
	FOR EACH ROW
	EXECUTE FUNCTION validate_session_organization_context();

CREATE TABLE IF NOT EXISTS organization_impersonation_sessions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	operator_session_id uuid NOT NULL,
	operator_user_id uuid NOT NULL,
	target_organization_id uuid NOT NULL,
	target_user_id uuid,
	reason text NOT NULL,
	ticket_reference varchar(150),
	started_at timestamp without time zone NOT NULL DEFAULT now(),
	expires_at timestamp without time zone NOT NULL,
	stopped_at timestamp without time zone,
	stopped_by_user_id uuid,
	stop_reason text,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT organization_impersonation_reason_check CHECK (
		char_length(btrim(reason)) > 0
	),
	CONSTRAINT organization_impersonation_window_check CHECK (
		expires_at > started_at
	),
	CONSTRAINT organization_impersonation_stop_check CHECK (
		(stopped_at IS NULL AND stopped_by_user_id IS NULL AND stop_reason IS NULL)
		OR (
			stopped_at IS NOT NULL
			AND stopped_by_user_id IS NOT NULL
			AND stopped_at >= started_at
			AND char_length(btrim(COALESCE(stop_reason, ''))) > 0
		)
	),
	CONSTRAINT organization_impersonation_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_organization_impersonation_operator_session_id
		FOREIGN KEY (operator_session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_impersonation_operator_user_id
		FOREIGN KEY (operator_user_id) REFERENCES users(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_impersonation_target_organization_id
		FOREIGN KEY (target_organization_id) REFERENCES organizations(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_impersonation_target_user_id
		FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_impersonation_stopped_by_user_id
		FOREIGN KEY (stopped_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_impersonation_active_operator_session
	ON organization_impersonation_sessions(operator_session_id)
	WHERE stopped_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organization_impersonation_target_time
	ON organization_impersonation_sessions(target_organization_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_organization_impersonation_operator_time
	ON organization_impersonation_sessions(operator_user_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_organization_impersonation_expiry
	ON organization_impersonation_sessions(expires_at)
	WHERE stopped_at IS NULL;

CREATE OR REPLACE FUNCTION validate_organization_impersonation_session()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	operator_session_expires_at timestamp without time zone;
BEGIN
	SELECT expires_at
	INTO operator_session_expires_at
	FROM sessions
	WHERE id = NEW.operator_session_id
		AND user_id = NEW.operator_user_id
		AND revoked_at IS NULL
		AND expires_at > now();

	IF NOT FOUND THEN
		RAISE EXCEPTION 'impersonation requires an active session owned by the operator';
	END IF;

	IF NEW.expires_at > operator_session_expires_at THEN
		RAISE EXCEPTION 'impersonation cannot outlive the operator session';
	END IF;

	IF NEW.target_user_id IS NOT NULL AND NOT EXISTS (
		SELECT 1
		FROM organization_memberships
		WHERE organization_id = NEW.target_organization_id
			AND user_id = NEW.target_user_id
			AND status = 'active'
			AND removed_at IS NULL
	) THEN
		RAISE EXCEPTION 'target user must be an active member of the target organization';
	END IF;

	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_validate_organization_impersonation_session
	BEFORE INSERT OR UPDATE OF
		operator_session_id,
		operator_user_id,
		target_organization_id,
		target_user_id,
		expires_at
	ON organization_impersonation_sessions
	FOR EACH ROW
	EXECUTE FUNCTION validate_organization_impersonation_session();
