CREATE TABLE IF NOT EXISTS user_permissions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	permission_id uuid NOT NULL,
	organization_id uuid,
	effect varchar(20) NOT NULL,
	assigned_by uuid,
	assigned_at timestamp without time zone NOT NULL DEFAULT now(),
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT user_permissions_effect_check CHECK (effect IN ('allow', 'deny')),
	CONSTRAINT fk_user_permissions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	CONSTRAINT fk_user_permissions_permission_id FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
	CONSTRAINT fk_user_permissions_assigned_by FOREIGN KEY (assigned_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_permissions_global_unique
	ON user_permissions(user_id, permission_id)
	WHERE organization_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_permissions_organization_unique
	ON user_permissions(user_id, permission_id, organization_id)
	WHERE organization_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_permissions_user_id ON user_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_permissions_permission_id ON user_permissions(permission_id);
CREATE INDEX IF NOT EXISTS idx_user_permissions_organization_id ON user_permissions(organization_id);
CREATE INDEX IF NOT EXISTS idx_user_permissions_assigned_by ON user_permissions(assigned_by);
