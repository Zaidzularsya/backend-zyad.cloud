WITH seed_templates (code, channel, locale) AS (
	VALUES
		('auth.password_reset', 'email', 'id-ID'),
		('user.invitation', 'email', 'id-ID'),
		('lead.created', 'whatsapp', 'id-ID'),
		('payment.paid', 'email', 'id-ID'),
		('security.new_login', 'email', 'id-ID'),
		('permission.updated', 'email', 'id-ID')
)
DELETE FROM notification_templates nt
USING seed_templates st
WHERE nt.code = st.code
	AND nt.channel = st.channel
	AND nt.locale = st.locale
	AND nt.version = 1
	AND nt.is_system = true;
