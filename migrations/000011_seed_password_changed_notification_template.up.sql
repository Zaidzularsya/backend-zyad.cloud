INSERT INTO notification_templates (
	code,
	name,
	description,
	channel,
	locale,
	subject_template,
	body_template,
	available_variables,
	sample_payload,
	status,
	is_system,
	is_active,
	version,
	created_at,
	updated_at
)
VALUES (
	'auth.password_changed',
	'Password Changed',
	'Security notification sent after a successful password reset',
	'email',
	'id-ID',
	'Password {{app_name}} berhasil diubah',
	'Hi {{user_name}}, password akun {{app_name}} Anda berhasil diubah pada {{changed_at}}. Jika Anda tidak melakukan perubahan ini, segera hubungi administrator.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Admin"},
		{"key":"changed_at","description":"Password change timestamp","required":true,"example":"2026-06-12 10:00 WIB"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Admin",
		"changed_at":"2026-06-12 10:00 WIB"
	}'::jsonb,
	'active',
	true,
	true,
	1,
	now(),
	now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name,
	description = EXCLUDED.description,
	subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template,
	available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload,
	status = 'active',
	is_system = true,
	is_active = true,
	updated_at = now()
WHERE notification_templates.is_system = true;
