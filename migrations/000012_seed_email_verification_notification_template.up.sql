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
	'auth.email_verification',
	'Email Verification',
	'Sent to verify a user''s email address',
	'email',
	'id-ID',
	'Verifikasi email Anda untuk {{app_name}}',
	'Hi {{user_name}}, silakan verifikasi alamat email Anda untuk {{app_name}} dengan mengeklik link berikut: {{verification_url}}. Link ini berlaku sampai {{expired_at}}.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Admin"},
		{"key":"verification_url","description":"Email verification URL","required":true,"example":"https://app.example.test/verify-email?token=abcdef"},
		{"key":"expired_at","description":"Link expiration time","required":true,"example":"2026-06-12 10:00 WIB"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Admin",
		"verification_url":"https://app.example.test/verify-email?token=abcdef",
		"expired_at":"2026-06-12 10:00 WIB"
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
