DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.slug = 'landing.page_template.manage';
DELETE FROM permissions WHERE slug = 'landing.page_template.manage';
DROP TABLE IF EXISTS landing_page_templates;
