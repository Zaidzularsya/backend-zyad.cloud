WITH finance_permissions(permission_name, action, description) AS (
	VALUES
		('platform.finance.coa.read', 'coa_read', 'Read finance chart of accounts and fiscal calendar'),
		('platform.finance.coa.manage', 'coa_manage', 'Manage finance chart of accounts, fiscal years, and periods'),
		('platform.finance.journal.read', 'journal_read', 'Read finance journal entries'),
		('platform.finance.journal.manage', 'journal_manage', 'Create, post, and reverse finance journal entries'),
		('platform.finance.reports.view', 'reports_view', 'View finance reports (general ledger, trial balance, profit & loss, balance sheet)')
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
		'finance',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM finance_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id, permission_name
),
platform_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'super_admin'
		OR slug = 'super_admin'
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	platform_roles.id,
	upserted_permissions.id,
	'all',
	now()
FROM platform_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
