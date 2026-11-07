DELETE FROM notification_templates WHERE code = 'mikrotik.isolation_task_failed' AND is_system = true;

DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name IN ('mikrotik_task.read', 'mikrotik_task.retry');
DELETE FROM permissions WHERE permission_name IN ('mikrotik_task.read', 'mikrotik_task.retry');
