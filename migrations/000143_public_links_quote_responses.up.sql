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
