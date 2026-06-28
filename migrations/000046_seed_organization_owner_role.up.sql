INSERT INTO roles (
	role_name,
	slug,
	description,
	is_system,
	created_at,
	updated_at
)
VALUES (
	'organization_owner',
	'organization_owner',
	'Owner role for customer organization workspaces',
	true,
	now(),
	now()
)
ON CONFLICT (slug)
DO UPDATE SET
	role_name = EXCLUDED.role_name,
	description = EXCLUDED.description,
	is_system = EXCLUDED.is_system,
	updated_at = now();

WITH owner_role AS (
	SELECT id
	FROM roles
	WHERE slug = 'organization_owner'
	LIMIT 1
),
owner_permissions AS (
	SELECT id
	FROM permissions
	WHERE slug IN (
		'organization.read',
		'organization.update',
		'organization.member.read',
		'organization.member.manage',
		'organization.domain.manage',
		'organization.feature.read',
		'landing.page.read',
		'landing.page.create',
		'landing.page.update',
		'landing.page.delete',
		'landing.page.restore',
		'landing.page.archive',
		'landing.page.publish',
		'landing.seo.manage',
		'landing.preview',
		'landing.section.manage',
		'landing.form.manage',
		'landing.submission.read',
		'landing.submission.update',
		'landing.submission.delete',
		'landing.submission.export',
		'landing.branding.read',
		'landing.branding.update',
		'landing.theme.manage',
		'landing.cta.manage',
		'landing.section_template.manage',
		'landing.media.manage',
		'landing.menu.manage',
		'landing.domain.read',
		'landing.domain.manage',
		'landing.integration.read',
		'landing.integration.manage'
	)
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	owner_role.id,
	owner_permissions.id,
	'organization',
	now()
FROM owner_role
CROSS JOIN owner_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
