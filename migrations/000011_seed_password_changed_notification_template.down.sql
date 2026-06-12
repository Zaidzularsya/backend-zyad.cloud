DELETE FROM notification_templates
WHERE code = 'auth.password_changed'
	AND channel = 'email'
	AND locale = 'id-ID'
	AND version = 1
	AND is_system = true;
