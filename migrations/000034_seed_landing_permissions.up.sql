WITH landing_permissions(permission_name, action, description) AS (
	VALUES
		('landing.page.read', 'read', 'Read landing pages'),
		('landing.page.create', 'create', 'Create new landing pages'),
		('landing.page.update', 'update', 'Update existing landing pages'),
		('landing.page.delete', 'delete', 'Delete landing pages'),
		('landing.page.restore', 'restore', 'Restore soft-deleted landing pages'),
		('landing.page.archive', 'archive', 'Archive landing pages'),
		('landing.page.publish', 'publish', 'Publish or unpublish landing pages'),
		('landing.seo.manage', 'manage_seo', 'Manage landing page SEO settings'),
		('landing.preview', 'preview', 'Generate and view landing page previews'),
		('landing.section.manage', 'manage_section', 'Manage landing page sections'),
		('landing.form.manage', 'manage_form', 'Manage landing page forms and fields'),
		('landing.submission.read', 'read_submission', 'Read landing page form submissions'),
		('landing.submission.update', 'update_submission', 'Update landing page form submission status and notes'),
		('landing.submission.delete', 'delete_submission', 'Delete landing page form submissions'),
		('landing.submission.export', 'export_submission', 'Export landing page form submissions'),
		('landing.branding.read', 'read_branding', 'Read landing page branding settings'),
		('landing.branding.update', 'update_branding', 'Update landing page branding settings'),
		('landing.theme.manage', 'manage_theme', 'Manage landing page themes'),
		('landing.cta.manage', 'manage_cta', 'Manage reusable Call to Actions'),
		('landing.section_template.manage', 'manage_section_template', 'Manage reusable section templates'),
		('landing.media.manage', 'manage_media', 'Manage landing page media assets'),
		('landing.menu.manage', 'manage_menu', 'Manage landing page navigation menus'),
		('landing.domain.read', 'read_domain', 'Read organization domain bindings'),
		('landing.domain.manage', 'manage_domain', 'Manage organization domain bindings'),
		('landing.integration.read', 'read_integration', 'Read lead integrations'),
		('landing.integration.manage', 'manage_integration', 'Manage lead integrations')
),
upserted_permissions AS (
	INSERT INTO permissions (
		permission_name,
		module,
		action,
		name,
		slug,
		description,
		created_at,
		updated_at
	)
	SELECT
		permission_name,
		'landing',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM landing_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
target_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('super_admin', 'organization_owner')
		OR slug IN ('super_admin', 'organization_owner')
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	target_roles.id,
	upserted_permissions.id,
	'organization',
	now()
FROM target_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
