-- Owner emails for payout bank account lifecycle (events receivable.payout_account_*, internal/modules/receivable).
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
)
VALUES (
	'receivable.payout_account_submitted',
	'Payout Account Submitted',
	'Sent to organization owners when a payout bank account is submitted (also a security alert)',
	'email',
	'id-ID',
	'Pengajuan rekening pencairan {{organization_name}}',
	'Hi {{user_name}}, rekening pencairan {{bank_name}} {{account_number_masked}} a.n. {{account_holder_name}} diajukan untuk {{organization_name}} di {{app_name}} dan menunggu verifikasi. Jika bukan Anda atau tim Anda yang mengajukan, segera batalkan pengajuan di {{settlement_url}} dan ganti password Anda.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"organization_name","description":"Organization name","required":true,"example":"PT Contoh"},
		{"key":"bank_name","description":"Bank name","required":true,"example":"Bank Central Asia (BCA)"},
		{"key":"account_number_masked","description":"Masked account number","required":true,"example":"••••7890"},
		{"key":"account_holder_name","description":"Account holder name","required":true,"example":"Budi Santoso"},
		{"key":"review_note","description":"Reviewer note (optional)","required":false,"example":"Nama pemilik tidak sesuai"},
		{"key":"settlement_url","description":"Bank account page URL","required":true,"example":"https://app.example.test/app/account/bank"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Sari",
		"organization_name":"PT Contoh",
		"bank_name":"Bank Central Asia (BCA)",
		"account_number_masked":"••••7890",
		"account_holder_name":"Budi Santoso",
		"review_note":"Nama pemilik tidak sesuai",
		"settlement_url":"https://app.example.test/app/account/bank"
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

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
)
VALUES (
	'receivable.payout_account_verified',
	'Payout Account Verified',
	'Sent to organization owners when a payout bank account is verified',
	'email',
	'id-ID',
	'Rekening pencairan {{organization_name}} terverifikasi',
	'Hi {{user_name}}, rekening pencairan {{bank_name}} {{account_number_masked}} a.n. {{account_holder_name}} untuk {{organization_name}} telah diverifikasi di {{app_name}} dan akan dipakai untuk setoran berikutnya. Detail: {{settlement_url}}. Catatan: {{review_note}}',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"organization_name","description":"Organization name","required":true,"example":"PT Contoh"},
		{"key":"bank_name","description":"Bank name","required":true,"example":"Bank Central Asia (BCA)"},
		{"key":"account_number_masked","description":"Masked account number","required":true,"example":"••••7890"},
		{"key":"account_holder_name","description":"Account holder name","required":true,"example":"Budi Santoso"},
		{"key":"review_note","description":"Reviewer note (optional)","required":false,"example":"Nama pemilik tidak sesuai"},
		{"key":"settlement_url","description":"Bank account page URL","required":true,"example":"https://app.example.test/app/account/bank"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Sari",
		"organization_name":"PT Contoh",
		"bank_name":"Bank Central Asia (BCA)",
		"account_number_masked":"••••7890",
		"account_holder_name":"Budi Santoso",
		"review_note":"Nama pemilik tidak sesuai",
		"settlement_url":"https://app.example.test/app/account/bank"
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

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
)
VALUES (
	'receivable.payout_account_rejected',
	'Payout Account Rejected',
	'Sent to organization owners when a payout bank account submission is rejected',
	'email',
	'id-ID',
	'Pengajuan rekening pencairan {{organization_name}} ditolak',
	'Hi {{user_name}}, pengajuan rekening pencairan {{bank_name}} {{account_number_masked}} a.n. {{account_holder_name}} untuk {{organization_name}} di {{app_name}} ditolak. Alasan: {{review_note}}. Silakan ajukan ulang di {{settlement_url}}.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"organization_name","description":"Organization name","required":true,"example":"PT Contoh"},
		{"key":"bank_name","description":"Bank name","required":true,"example":"Bank Central Asia (BCA)"},
		{"key":"account_number_masked","description":"Masked account number","required":true,"example":"••••7890"},
		{"key":"account_holder_name","description":"Account holder name","required":true,"example":"Budi Santoso"},
		{"key":"review_note","description":"Reviewer note (optional)","required":false,"example":"Nama pemilik tidak sesuai"},
		{"key":"settlement_url","description":"Bank account page URL","required":true,"example":"https://app.example.test/app/account/bank"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Sari",
		"organization_name":"PT Contoh",
		"bank_name":"Bank Central Asia (BCA)",
		"account_number_masked":"••••7890",
		"account_holder_name":"Budi Santoso",
		"review_note":"Nama pemilik tidak sesuai",
		"settlement_url":"https://app.example.test/app/account/bank"
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

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
)
VALUES (
	'receivable.payout_account_assigned',
	'Payout Account Assigned',
	'Sent to organization owners when an administrator assigns the payout bank account',
	'email',
	'id-ID',
	'Rekening pencairan {{organization_name}} ditetapkan admin',
	'Hi {{user_name}}, admin {{app_name}} menetapkan rekening pencairan {{bank_name}} {{account_number_masked}} a.n. {{account_holder_name}} untuk {{organization_name}}. Rekening ini langsung aktif untuk setoran berikutnya. Detail: {{settlement_url}}. Catatan: {{review_note}}',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"organization_name","description":"Organization name","required":true,"example":"PT Contoh"},
		{"key":"bank_name","description":"Bank name","required":true,"example":"Bank Central Asia (BCA)"},
		{"key":"account_number_masked","description":"Masked account number","required":true,"example":"••••7890"},
		{"key":"account_holder_name","description":"Account holder name","required":true,"example":"Budi Santoso"},
		{"key":"review_note","description":"Reviewer note (optional)","required":false,"example":"Nama pemilik tidak sesuai"},
		{"key":"settlement_url","description":"Bank account page URL","required":true,"example":"https://app.example.test/app/account/bank"}
	]'::jsonb,
	'{
		"app_name":"Zyad Cloud",
		"user_name":"Sari",
		"organization_name":"PT Contoh",
		"bank_name":"Bank Central Asia (BCA)",
		"account_number_masked":"••••7890",
		"account_holder_name":"Budi Santoso",
		"review_note":"Nama pemilik tidak sesuai",
		"settlement_url":"https://app.example.test/app/account/bank"
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

