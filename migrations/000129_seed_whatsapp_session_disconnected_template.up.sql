-- Email sent to organization owners when a connected WhatsApp session drops
-- (event whatsapp.session_disconnected, internal/modules/whatsapp).
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
	'whatsapp.session_disconnected',
	'WhatsApp Session Disconnected',
	'Sent to organization owners when a connected WhatsApp number stops working',
	'email',
	'id-ID',
	'WhatsApp {{session_name}} terputus',
	'Hi {{user_name}}, nomor WhatsApp {{session_name}} di {{app_name}} terputus pada {{occurred_at}} (status: {{status}}). Pesan masuk tidak akan diterima sampai nomor dihubungkan kembali. Buka {{connections_url}} untuk menghubungkan ulang.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Admin"},
		{"key":"session_name","description":"WhatsApp session label (name and number)","required":true,"example":"Sales +6281234567890"},
		{"key":"status","description":"Current session status","required":true,"example":"gagal"},
		{"key":"occurred_at","description":"When the disconnect was observed","required":true,"example":"2026-09-24 10:00 WIB"},
		{"key":"connections_url","description":"WhatsApp connections page URL","required":true,"example":"https://app.example.test/app/whatsapp"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Admin",
		"session_name":"Sales +6281234567890",
		"status":"gagal",
		"occurred_at":"2026-09-24 10:00 WIB",
		"connections_url":"https://app.example.test/app/whatsapp"
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
