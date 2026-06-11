ALTER TABLE roles
	ADD COLUMN IF NOT EXISTS slug varchar(150),
	ADD COLUMN IF NOT EXISTS is_system boolean NOT NULL DEFAULT false;

UPDATE roles
SET slug = lower(regexp_replace(role_name, '[^a-zA-Z0-9]+', '_', 'g'))
WHERE slug IS NULL OR slug = '';

ALTER TABLE roles
	ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug_unique ON roles(slug);

ALTER TABLE permissions
	ADD COLUMN IF NOT EXISTS module varchar(100),
	ADD COLUMN IF NOT EXISTS action varchar(100),
	ADD COLUMN IF NOT EXISTS name varchar(150),
	ADD COLUMN IF NOT EXISTS slug varchar(150);

UPDATE permissions
SET
	name = COALESCE(NULLIF(name, ''), permission_name),
	slug = COALESCE(NULLIF(slug, ''), permission_name),
	module = COALESCE(
		NULLIF(module, ''),
		CASE
			WHEN position('.' IN permission_name) > 0 THEN split_part(permission_name, '.', 1)
			WHEN position(':' IN permission_name) > 0 THEN split_part(permission_name, ':', 1)
			ELSE 'permission'
		END
	),
	action = COALESCE(
		NULLIF(action, ''),
		CASE
			WHEN position('.' IN permission_name) > 0 THEN split_part(permission_name, '.', 2)
			WHEN position(':' IN permission_name) > 0 THEN split_part(permission_name, ':', 2)
			ELSE 'manage'
		END
	);

ALTER TABLE permissions
	ALTER COLUMN module SET NOT NULL,
	ALTER COLUMN action SET NOT NULL,
	ALTER COLUMN name SET NOT NULL,
	ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_permissions_slug_unique ON permissions(slug);
CREATE INDEX IF NOT EXISTS idx_permissions_module_action ON permissions(module, action);

ALTER TABLE user_roles
	ADD COLUMN IF NOT EXISTS id uuid DEFAULT gen_random_uuid(),
	ADD COLUMN IF NOT EXISTS organization_id uuid,
	ADD COLUMN IF NOT EXISTS assigned_by uuid;

UPDATE user_roles
SET id = gen_random_uuid()
WHERE id IS NULL;

ALTER TABLE user_roles
	ALTER COLUMN id SET NOT NULL;

ALTER TABLE user_roles
	DROP CONSTRAINT IF EXISTS user_roles_pkey;

ALTER TABLE user_roles
	ADD CONSTRAINT user_roles_pkey PRIMARY KEY (id),
	ADD CONSTRAINT user_roles_user_id_role_id_unique UNIQUE (user_id, role_id);

ALTER TABLE user_roles
	ADD CONSTRAINT fk_user_roles_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT fk_user_roles_assigned_by FOREIGN KEY (assigned_by) REFERENCES users(id) ON DELETE SET NULL NOT VALID;

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_organization_id ON user_roles(organization_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_assigned_by ON user_roles(assigned_by);
