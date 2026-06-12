WITH seed_templates (
	code,
	name,
	description,
	channel,
	locale,
	subject_template,
	body_template,
	available_variables,
	sample_payload
) AS (
	VALUES
	(
		'auth.password_reset',
		'Password Reset',
		'Default email template for password reset requests',
		'email',
		'id-ID',
		'Reset password {{app_name}}',
		'Hi {{user_name}}, use this link to reset your {{app_name}} password: {{reset_url}}. This link expires at {{expired_at}}.',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"user_name","description":"Recipient user name","required":true,"example":"Admin"},
			{"key":"reset_url","description":"Password reset URL","required":true,"example":"https://app.example.test/reset"},
			{"key":"expired_at","description":"Reset link expiration time","required":true,"example":"2026-06-12 10:00 WIB"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"user_name":"Admin",
			"reset_url":"https://app.example.test/reset",
			"expired_at":"2026-06-12 10:00 WIB"
		}'::jsonb
	),
	(
		'user.invitation',
		'User Invitation',
		'Default email template for organization invitations',
		'email',
		'id-ID',
		'Invitation to join {{organization_name}}',
		'Hi {{invitee_email}}, {{inviter_name}} invited you to join {{organization_name}} on {{app_name}}. Accept here: {{invitation_url}}. This invitation expires at {{expired_at}}.',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"inviter_name","description":"User who sent the invitation","required":true,"example":"Owner"},
			{"key":"invitee_email","description":"Invited user email","required":true,"example":"member@example.test"},
			{"key":"organization_name","description":"Organization name","required":true,"example":"Acme Network"},
			{"key":"invitation_url","description":"Invitation accept URL","required":true,"example":"https://app.example.test/invitations/accept"},
			{"key":"expired_at","description":"Invitation expiration time","required":true,"example":"2026-06-19 10:00 WIB"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"inviter_name":"Owner",
			"invitee_email":"member@example.test",
			"organization_name":"Acme Network",
			"invitation_url":"https://app.example.test/invitations/accept",
			"expired_at":"2026-06-19 10:00 WIB"
		}'::jsonb
	),
	(
		'lead.created',
		'Lead Created',
		'Default WhatsApp template for new lead alerts',
		'whatsapp',
		'id-ID',
		NULL,
		'New lead on {{app_name}}: {{lead_name}} from {{lead_source}}. Phone: {{lead_phone}}. Assigned to {{assigned_sales_name}}. Open: {{crm_url}}',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"lead_name","description":"Lead name","required":true,"example":"Budi"},
			{"key":"lead_source","description":"Lead source","required":true,"example":"landing_page"},
			{"key":"lead_phone","description":"Lead phone number","required":false,"example":"+628123456789"},
			{"key":"assigned_sales_name","description":"Assigned sales name","required":false,"example":"Sales Team"},
			{"key":"crm_url","description":"CRM lead detail URL","required":true,"example":"https://app.example.test/admin/leads/123"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"lead_name":"Budi",
			"lead_source":"landing_page",
			"lead_phone":"+628123456789",
			"assigned_sales_name":"Sales Team",
			"crm_url":"https://app.example.test/admin/leads/123"
		}'::jsonb
	),
	(
		'payment.paid',
		'Payment Paid',
		'Default email template for paid payment notifications',
		'email',
		'id-ID',
		'Payment received for {{invoice_number}}',
		'Hi {{customer_name}}, we received your payment {{amount}} for invoice {{invoice_number}} on {{paid_at}}. Thank you for using {{app_name}}.',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"customer_name","description":"Customer name","required":true,"example":"Budi"},
			{"key":"invoice_number","description":"Invoice number","required":true,"example":"INV-2026-0001"},
			{"key":"amount","description":"Paid amount","required":true,"example":"Rp150.000"},
			{"key":"paid_at","description":"Payment timestamp","required":true,"example":"2026-06-12 10:00 WIB"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"customer_name":"Budi",
			"invoice_number":"INV-2026-0001",
			"amount":"Rp150.000",
			"paid_at":"2026-06-12 10:00 WIB"
		}'::jsonb
	),
	(
		'security.new_login',
		'New Login Alert',
		'Default email template for new login alerts',
		'email',
		'id-ID',
		'New login to {{app_name}}',
		'Hi {{user_name}}, a new login to {{app_name}} happened at {{login_at}} from IP {{ip_address}} using {{device_name}}.',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"user_name","description":"Recipient user name","required":true,"example":"Admin"},
			{"key":"login_at","description":"Login timestamp","required":true,"example":"2026-06-12 10:00 WIB"},
			{"key":"ip_address","description":"Login IP address","required":true,"example":"127.0.0.1"},
			{"key":"device_name","description":"Device or browser name","required":false,"example":"Chrome on Linux"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"user_name":"Admin",
			"login_at":"2026-06-12 10:00 WIB",
			"ip_address":"127.0.0.1",
			"device_name":"Chrome on Linux"
		}'::jsonb
	),
	(
		'permission.updated',
		'Permission Updated',
		'Default email template for permission update alerts',
		'email',
		'id-ID',
		'Permission updated: {{permission_name}}',
		'Hi {{user_name}}, {{actor_name}} updated permission {{permission_name}} on {{app_name}} at {{updated_at}}.',
		'[
			{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
			{"key":"user_name","description":"Affected user name","required":true,"example":"Admin"},
			{"key":"actor_name","description":"User who changed the permission","required":true,"example":"Super Admin"},
			{"key":"permission_name","description":"Permission name","required":true,"example":"notification_template.update"},
			{"key":"updated_at","description":"Permission update timestamp","required":true,"example":"2026-06-12 10:00 WIB"}
		]'::jsonb,
		'{
			"app_name":"Zyad Cloud",
			"user_name":"Admin",
			"actor_name":"Super Admin",
			"permission_name":"notification_template.update",
			"updated_at":"2026-06-12 10:00 WIB"
		}'::jsonb
	)
)
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
SELECT
	code,
	name,
	description,
	channel,
	locale,
	subject_template,
	body_template,
	available_variables,
	sample_payload,
	'active',
	true,
	true,
	1,
	now(),
	now()
FROM seed_templates
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
