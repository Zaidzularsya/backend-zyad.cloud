-- Email ke pembeli saat invoice aktivasi halaman lunas dan halaman terbit otomatis.
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'landing.activation_paid', 'Landing Activation Paid', 'Sent to the buyer when a page activation invoice is paid and the page is published', 'email', 'id-ID',
	'Pembayaran diterima, halaman {{page_title}} sudah aktif',
	'Hi {{buyer_name}}, pembayaran Anda sudah kami terima dan halaman {{page_title}} sudah terbit. Masa aktif halaman sampai {{active_until}}. {{page_link_line}}',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"buyer_name","description":"Buyer name","required":true,"example":"Budi"},
		{"key":"page_title","description":"Activated page title","required":true,"example":"Toko Budi"},
		{"key":"active_until","description":"End of the paid period","required":true,"example":"17 Okt 2026"},
		{"key":"page_link_line","description":"Sentence with the public page link (empty when no host is available)","required":false,"example":"Halaman Anda dapat dibuka di https://klien.example.test/toko-budi."}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","buyer_name":"Budi","page_title":"Toko Budi","active_until":"17 Okt 2026","page_link_line":"Halaman Anda dapat dibuka di https://klien.example.test/toko-budi."}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
