-- Indeks parsial untuk job suspend otomatis: invoice overdue per organisasi diurut jatuh tempo.
CREATE INDEX IF NOT EXISTS idx_receivable_invoices_overdue_due
	ON receivable_invoices(organization_id, due_date) WHERE status = 'overdue';

-- Notifikasi ke owner workspace saat ditangguhkan karena tagihan dan saat aktif kembali.
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'self_serve.workspace_suspended', 'Workspace Suspended (Billing)', 'Sent to workspace owners when the workspace is suspended because an invoice is overdue beyond the grace period', 'email', 'id-ID',
	'Workspace {{workspace_name}} ditangguhkan karena tagihan belum dibayar',
	'Hi {{user_name}}, workspace {{workspace_name}} ditangguhkan karena invoice {{invoice_number}} senilai {{amount_due}} yang jatuh tempo pada {{due_date}} belum dibayar (masa tenggang {{grace_days}} hari sudah lewat). Bayar sekarang di {{invoice_url}}. Workspace akan aktif kembali otomatis setelah tagihan lunas.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"workspace_name","description":"Workspace name","required":true,"example":"Toko Maju"},
		{"key":"invoice_number","description":"Overdue invoice number","required":true,"example":"INV-2026-0377"},
		{"key":"invoice_url","description":"Public invoice payment link","required":true,"example":"https://app.example.test/p/abc"},
		{"key":"amount_due","description":"Amount due (formatted)","required":true,"example":"Rp333.000"},
		{"key":"due_date","description":"Invoice due date","required":true,"example":"2026-09-10"},
		{"key":"grace_days","description":"Grace period in days","required":true,"example":"7"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","workspace_name":"Toko Maju","invoice_number":"INV-2026-0377","invoice_url":"https://app.example.test/p/abc","amount_due":"Rp333.000","due_date":"2026-09-10","grace_days":"7"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'self_serve.workspace_reactivated', 'Workspace Reactivated (Billing)', 'Sent to workspace owners when the workspace is reactivated after the overdue invoices are settled', 'email', 'id-ID',
	'Workspace {{workspace_name}} aktif kembali',
	'Hi {{user_name}}, pembayaran invoice {{invoice_number}} sudah kami terima. Workspace {{workspace_name}} aktif kembali dan bisa dipakai seperti biasa.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"workspace_name","description":"Workspace name","required":true,"example":"Toko Maju"},
		{"key":"invoice_number","description":"Settled invoice number","required":true,"example":"INV-2026-0377"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","workspace_name":"Toko Maju","invoice_number":"INV-2026-0377"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
