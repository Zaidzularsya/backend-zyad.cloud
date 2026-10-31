-- Permission tugas isolir (Fase 3) dan template notifikasi saat tugas gagal. Permission hanya organization_owner (ikuti 000172).
WITH mikrotik_task_permissions(permission_name, module, action, description) AS (
	VALUES
		('mikrotik_task.read', 'mikrotik', 'read', 'Read Mikrotik isolation tasks'),
		('mikrotik_task.retry', 'mikrotik', 'retry', 'Retry failed Mikrotik isolation tasks')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM mikrotik_task_permissions
	ON CONFLICT (slug)
	DO UPDATE SET permission_name = EXCLUDED.permission_name, module = EXCLUDED.module, action = EXCLUDED.action,
		name = EXCLUDED.name, slug = EXCLUDED.slug, description = EXCLUDED.description, updated_at = now()
	RETURNING id
),
owner_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT owner_roles.id, upserted_permissions.id, 'organization', now()
FROM owner_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'mikrotik.isolation_task_failed', 'Mikrotik Isolation Task Failed', 'Sent to the tenant admins when a Mikrotik isolate/restore task fails', 'email', 'id-ID',
	'Tugas {{action}} PPPoE {{secret_name}} gagal',
	'Hi {{user_name}}, tugas {{action}} untuk akun PPPoE {{secret_name}} pada router {{router_name}} gagal: {{reason}}. Buka {{tasks_url}} untuk meninjau dan mencoba ulang.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"router_name","description":"Router name","required":true,"example":"Router Pusat"},
		{"key":"secret_name","description":"PPP secret name","required":true,"example":"budi-001"},
		{"key":"action","description":"Task action label (isolir or pulihkan)","required":true,"example":"isolir"},
		{"key":"reason","description":"Failure reason (no provider internals)","required":true,"example":"Router tidak dapat dijangkau."},
		{"key":"tasks_url","description":"Isolation tasks page URL","required":true,"example":"https://app.example.test/app/mikrotik/tasks"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","router_name":"Router Pusat","secret_name":"budi-001","action":"isolir","reason":"Router tidak dapat dijangkau.","tasks_url":"https://app.example.test/app/mikrotik/tasks"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
