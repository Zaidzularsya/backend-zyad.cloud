-- Galeri template landing: tabel GLOBAL (tanpa organization_id, tanpa RLS).
-- Visibilitas dan hak tulis dijaga di PageTemplateService.
CREATE TABLE IF NOT EXISTS landing_page_templates (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	slug varchar(80) NOT NULL UNIQUE,
	name varchar(120) NOT NULL,
	description text NOT NULL DEFAULT '',
	category varchar(40) NOT NULL,
	sort_order integer NOT NULL DEFAULT 100,
	status varchar(20) NOT NULL DEFAULT 'draft',
	platform_only boolean NOT NULL DEFAULT false,
	html text NOT NULL,
	css text NOT NULL DEFAULT '',
	source varchar(10) NOT NULL,
	seed_version integer NULL,
	seed_modified boolean NOT NULL DEFAULT false,
	source_page_id uuid NULL REFERENCES landing_pages(id) ON DELETE SET NULL,
	created_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	updated_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	created_at timestamp NOT NULL DEFAULT now(),
	updated_at timestamp NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_templates_slug_check CHECK (slug ~ '^[a-z0-9-]+$'),
	CONSTRAINT landing_page_templates_category_check CHECK (category IN (
		'dasar', 'bisnis-jasa', 'personal', 'acara', 'kesehatan', 'pendidikan',
		'komunitas', 'kuliner', 'properti', 'ritel-umkm', 'teknologi-isp', 'platform'
	)),
	CONSTRAINT landing_page_templates_status_check CHECK (status IN ('draft', 'published', 'archived')),
	CONSTRAINT landing_page_templates_source_check CHECK (source IN ('seed', 'admin'))
);

CREATE INDEX IF NOT EXISTS landing_page_templates_status_sort_idx
	ON landing_page_templates (status, sort_order);

WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'landing.page_template.manage', 'landing', 'manage_page_template', 'landing.page_template.manage',
		'landing.page_template.manage', 'Manage landing page template gallery', now(), now()
	)
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
full_access_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner', 'super_admin')
		OR slug IN ('organization_owner', 'super_admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT full_access_roles.id, new_permission.id, 'organization', now()
FROM full_access_roles
CROSS JOIN new_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
