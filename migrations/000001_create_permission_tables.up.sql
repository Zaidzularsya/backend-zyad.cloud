CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS permissions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	permission_name varchar(150) NOT NULL,
	description text,
	module_id uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT permission_name_unique UNIQUE (permission_name)
);

CREATE TABLE IF NOT EXISTS roles (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	role_name varchar(150) NOT NULL,
	description text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT role_name_unique UNIQUE (role_name)
);

CREATE TABLE IF NOT EXISTS role_permissions (
	role_id uuid NOT NULL,
	permission_id uuid NOT NULL,
	scope varchar(50) NOT NULL DEFAULT 'organization',
	granted_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id),
	CONSTRAINT role_permissions_scope_check CHECK (
		scope IN ('none', 'own', 'team', 'branch', 'department', 'organization', 'all')
	),
	CONSTRAINT fk_role_permissions_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
	CONSTRAINT fk_role_permissions_permission_id FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_roles (
	user_id uuid NOT NULL,
	role_id uuid NOT NULL,
	assigned_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id),
	CONSTRAINT fk_user_roles_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_permissions_module_id ON permissions(module_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON role_permissions(permission_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);
