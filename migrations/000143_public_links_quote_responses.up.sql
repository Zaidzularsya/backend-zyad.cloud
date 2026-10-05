-- Directory token → dokumen. TIDAK memakai RLS: pencarian token terjadi
-- sebelum tenant diketahui (pola mailbox_directory, 000135). Semua operasi
-- ber-scope tetap memfilter organization_id secara eksplisit di repository.
CREATE TABLE IF NOT EXISTS public_links (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	document_type varchar(30) NOT NULL,
	document_id uuid NOT NULL,
	token_hash bytea NOT NULL,
	token_enc text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	revoked_at timestamp without time zone NULL,
	last_viewed_at timestamp without time zone NULL,
	view_count integer NOT NULL DEFAULT 0,
	created_by uuid NULL REFERENCES users(id) ON DELETE SET NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT public_links_document_type_check CHECK (document_type IN ('quotation', 'invoice'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_public_links_token_hash ON public_links(token_hash);
CREATE UNIQUE INDEX IF NOT EXISTS idx_public_links_active_document
	ON public_links(organization_id, document_type, document_id) WHERE revoked_at IS NULL;

ALTER TABLE crm_quotations DROP CONSTRAINT IF EXISTS crm_quotations_status_check;
ALTER TABLE crm_quotations ADD CONSTRAINT crm_quotations_status_check CHECK (
	status IN ('draft', 'sent', 'approved', 'rejected', 'expired', 'superseded', 'revision_requested'));

CREATE TABLE IF NOT EXISTS crm_quotation_responses (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	quotation_id uuid NOT NULL,
	link_id uuid NULL REFERENCES public_links(id) ON DELETE SET NULL,
	action varchar(20) NOT NULL,
	revision_categories text[] NULL,
	note text NULL,
	responder_name varchar(150) NOT NULL,
	ip inet NULL,
	user_agent varchar(500) NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_quotation_responses_action_check CHECK (action IN ('approved', 'revision_requested')),
	CONSTRAINT fk_crm_quotation_responses_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_quotation_responses_quotation FOREIGN KEY (organization_id, quotation_id)
		REFERENCES crm_quotations(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_crm_quotation_responses_quotation
	ON crm_quotation_responses(organization_id, quotation_id, created_at DESC);
SELECT apply_organization_rls('crm_quotation_responses'::regclass);

ALTER TABLE crm_activities ADD COLUMN IF NOT EXISTS metadata jsonb NULL;
ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;
ALTER TABLE crm_activities ADD CONSTRAINT crm_activities_type_check CHECK (
	type IN ('call', 'email', 'meeting', 'task', 'note', 'whatsapp', 'quotation_response'));

-- Notifikasi ke pemilik deal saat customer merespons penawaran lewat link publik.
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'crm.quotation_approved', 'CRM Quotation Approved by Customer', 'Sent to the deal owner when a customer approves a quotation via the public link', 'email', 'id-ID',
	'Penawaran {{quotation_number}} disetujui',
	'Hi {{user_name}}, {{responder_name}} menyetujui penawaran {{quotation_number}} lewat link penawaran. Buka {{deal_url}} untuk menindaklanjuti.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"quotation_number","description":"Quotation number","required":true,"example":"QUO-2026-0001"},
		{"key":"responder_name","description":"Name the customer entered when responding","required":true,"example":"Budi Santoso"},
		{"key":"deal_url","description":"Deal (or quotation) page URL","required":true,"example":"https://app.example.test/app/crm/deals/1"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","quotation_number":"QUO-2026-0001","responder_name":"Budi Santoso","deal_url":"https://app.example.test/app/crm/deals/1"}'::jsonb,
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
	'crm.quotation_revision_requested', 'CRM Quotation Revision Requested', 'Sent to the deal owner when a customer requests a quotation revision via the public link', 'email', 'id-ID',
	'Revisi diminta untuk {{quotation_number}}',
	'Hi {{user_name}}, {{responder_name}} meminta revisi penawaran {{quotation_number}}. Bagian yang perlu direvisi: {{categories}}. Catatan: {{note}}. Buka {{deal_url}} untuk menindaklanjuti.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"quotation_number","description":"Quotation number","required":true,"example":"QUO-2026-0001"},
		{"key":"responder_name","description":"Name the customer entered when responding","required":true,"example":"Budi Santoso"},
		{"key":"categories","description":"Requested revision categories (labels, comma separated)","required":true,"example":"Harga/diskon, Lainnya"},
		{"key":"note","description":"Customer note (plain text, optional)","required":false,"example":"Mohon diskon tambahan"},
		{"key":"deal_url","description":"Deal (or quotation) page URL","required":true,"example":"https://app.example.test/app/crm/deals/1"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","quotation_number":"QUO-2026-0001","responder_name":"Budi Santoso","categories":"Harga/diskon, Lainnya","note":"Mohon diskon tambahan","deal_url":"https://app.example.test/app/crm/deals/1"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
