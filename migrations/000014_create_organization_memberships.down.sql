DROP TRIGGER IF EXISTS trg_increment_membership_version_for_role ON user_roles;
DROP FUNCTION IF EXISTS increment_membership_version_for_role();

DROP TRIGGER IF EXISTS trg_increment_membership_version ON organization_memberships;
DROP FUNCTION IF EXISTS increment_membership_version();

ALTER TABLE user_roles
	DROP CONSTRAINT IF EXISTS fk_user_roles_organization_id;

DROP INDEX IF EXISTS idx_user_roles_organization_unique;
DROP INDEX IF EXISTS idx_user_roles_global_unique;

WITH ranked_user_roles AS (
	SELECT
		id,
		row_number() OVER (
			PARTITION BY user_id, role_id
			ORDER BY organization_id NULLS FIRST, assigned_at DESC, id
		) AS duplicate_rank
	FROM user_roles
)
DELETE FROM user_roles
WHERE id IN (
	SELECT id
	FROM ranked_user_roles
	WHERE duplicate_rank > 1
);

ALTER TABLE user_roles
	ADD CONSTRAINT user_roles_user_id_role_id_unique UNIQUE (user_id, role_id);

DROP TABLE IF EXISTS organization_memberships;
