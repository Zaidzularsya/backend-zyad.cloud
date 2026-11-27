-- Staff-sent customer claim invite email (event social.customer_claim_invite, internal/modules/social).
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
)
VALUES (
	'social.customer_claim_invite',
	'Customer Claim Invite',
	'Sent to a CRM contact when tenant staff invite them to link their account as a customer',
	'email',
	'id-ID',
	'Undangan menautkan akun pelanggan {{organization_name}}',
	'Hi {{contact_name}}, {{organization_name}} mengundang Anda menautkan akun {{app_name}} sebagai pelanggan mereka. Buka tautan berikut untuk menerima undangan: {{claim_url}}. Tautan hanya bisa dipakai sekali dan berlaku sampai {{expired_at}}. Jika Anda tidak merasa menjadi pelanggan {{organization_name}}, abaikan email ini.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"organization_name","description":"Organization (provider) name","required":true,"example":"PT Contoh"},
		{"key":"contact_name","description":"Invited CRM contact name","required":true,"example":"Budi Santoso"},
		{"key":"claim_url","description":"Claim invite link","required":true,"example":"https://app.example.test/claim?token=abc"},
		{"key":"expired_at","description":"Invite expiry time","required":true,"example":"04-12-2026 10:00 WIB"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"organization_name":"PT Contoh",
		"contact_name":"Budi Santoso",
		"claim_url":"https://app.example.test/claim?token=abc",
		"expired_at":"04-12-2026 10:00 WIB"
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
