CREATE TABLE IF NOT EXISTS organization_memberships (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	user_id uuid NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'invited',
	is_owner boolean NOT NULL DEFAULT false,
	version bigint NOT NULL DEFAULT 1,
	invited_by uuid,
	invited_email varchar(255),
	invitation_token_hash text,
	invitation_expires_at timestamp without time zone,
	invited_at timestamp without time zone,
	accepted_at timestamp without time zone,
	suspended_at timestamp without time zone,
	removed_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT organization_memberships_status_check CHECK (
		status IN ('invited', 'active', 'suspended', 'removed')
	),
	CONSTRAINT organization_memberships_version_check CHECK (version > 0),
	CONSTRAINT organization_memberships_user_organization_unique UNIQUE (
		user_id,
		organization_id
	),
	CONSTRAINT fk_organization_memberships_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_memberships_user_id
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	CONSTRAINT fk_organization_memberships_invited_by
		FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_organization_memberships_organization_status
	ON organization_memberships(organization_id, status);

CREATE INDEX IF NOT EXISTS idx_organization_memberships_user_status
	ON organization_memberships(user_id, status);

CREATE INDEX IF NOT EXISTS idx_organization_memberships_active_lookup
	ON organization_memberships(user_id, organization_id, version)
	WHERE status = 'active' AND removed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organization_memberships_owner
	ON organization_memberships(organization_id)
	WHERE is_owner = true AND status = 'active' AND removed_at IS NULL;

INSERT INTO organization_memberships (
	organization_id,
	user_id,
	status,
	version,
	accepted_at,
	created_at,
	updated_at
)
SELECT DISTINCT
	ur.organization_id,
	ur.user_id,
	'active',
	1,
	COALESCE(ur.assigned_at, now()),
	COALESCE(ur.assigned_at, now()),
	now()
FROM user_roles ur
JOIN organizations organization ON organization.id = ur.organization_id
WHERE ur.organization_id IS NOT NULL
ON CONFLICT (user_id, organization_id) DO NOTHING;

ALTER TABLE user_roles
	DROP CONSTRAINT IF EXISTS user_roles_user_id_role_id_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_global_unique
	ON user_roles(user_id, role_id)
	WHERE organization_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_roles_organization_unique
	ON user_roles(user_id, role_id, organization_id)
	WHERE organization_id IS NOT NULL;

ALTER TABLE user_roles
	ADD CONSTRAINT fk_user_roles_organization_id
	FOREIGN KEY (organization_id) REFERENCES organizations(id)
	ON DELETE CASCADE NOT VALID;

CREATE OR REPLACE FUNCTION increment_membership_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF NEW.status IS DISTINCT FROM OLD.status
		OR NEW.is_owner IS DISTINCT FROM OLD.is_owner THEN
		NEW.version := GREATEST(NEW.version, OLD.version + 1);
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_increment_membership_version
	BEFORE UPDATE ON organization_memberships
	FOR EACH ROW
	EXECUTE FUNCTION increment_membership_version();

CREATE OR REPLACE FUNCTION increment_membership_version_for_role()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	affected_user_id uuid;
	affected_organization_id uuid;
BEGIN
	IF TG_OP = 'DELETE' THEN
		affected_user_id := OLD.user_id;
		affected_organization_id := OLD.organization_id;
	ELSE
		affected_user_id := NEW.user_id;
		affected_organization_id := NEW.organization_id;
	END IF;

	IF affected_organization_id IS NOT NULL THEN
		UPDATE organization_memberships
		SET version = version + 1,
			updated_at = now()
		WHERE user_id = affected_user_id
			AND organization_id = affected_organization_id
			AND status <> 'removed';
	END IF;

	IF TG_OP = 'DELETE' THEN
		RETURN OLD;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_increment_membership_version_for_role
	AFTER INSERT OR DELETE ON user_roles
	FOR EACH ROW
	EXECUTE FUNCTION increment_membership_version_for_role();
