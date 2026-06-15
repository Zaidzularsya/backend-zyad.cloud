DELETE FROM notification_templates
WHERE code = 'auth.email_verification'
	AND channel = 'email'
	AND locale = 'id-ID'
	AND version = 1;
