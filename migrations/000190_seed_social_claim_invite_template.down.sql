DELETE FROM notification_templates
WHERE code = 'social.customer_claim_invite'
	AND channel = 'email'
	AND locale = 'id-ID'
	AND version = 1
	AND is_system = true;
