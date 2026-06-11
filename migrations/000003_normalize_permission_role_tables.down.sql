DROP INDEX IF EXISTS idx_user_roles_assigned_by;
DROP INDEX IF EXISTS idx_user_roles_organization_id;
DROP INDEX IF EXISTS idx_user_roles_user_id;

ALTER TABLE user_roles
	DROP CONSTRAINT IF EXISTS fk_user_roles_assigned_by,
	DROP CONSTRAINT IF EXISTS fk_user_roles_user_id,
	DROP CONSTRAINT IF EXISTS user_roles_user_id_role_id_unique,
	DROP CONSTRAINT IF EXISTS user_roles_pkey;

ALTER TABLE user_roles
	ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);

ALTER TABLE user_roles
	DROP COLUMN IF EXISTS assigned_by,
	DROP COLUMN IF EXISTS organization_id,
	DROP COLUMN IF EXISTS id;

DROP INDEX IF EXISTS idx_permissions_module_action;
DROP INDEX IF EXISTS idx_permissions_slug_unique;

ALTER TABLE permissions
	DROP COLUMN IF EXISTS slug,
	DROP COLUMN IF EXISTS name,
	DROP COLUMN IF EXISTS action,
	DROP COLUMN IF EXISTS module;

DROP INDEX IF EXISTS idx_roles_slug_unique;

ALTER TABLE roles
	DROP COLUMN IF EXISTS is_system,
	DROP COLUMN IF EXISTS slug;
