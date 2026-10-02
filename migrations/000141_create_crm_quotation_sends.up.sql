CREATE TABLE IF NOT EXISTS crm_quotation_sends (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	quotation_id uuid NOT NULL,
	channel varchar(20) NOT NULL,
	mode varchar(20) NOT NULL,
	recipient varchar(255) NOT NULL,
	pdf_asset_id uuid NULL,
	status varchar(20) NOT NULL,
	error text NULL,
	external_message_id varchar(255) NULL,
	client_request_id varchar(100) NULL,
	sent_by uuid NULL,
	sent_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_quotation_sends_channel_check CHECK (channel IN ('email', 'whatsapp')),
	CONSTRAINT crm_quotation_sends_mode_check CHECK (mode IN ('text', 'pdf', 'text_pdf')),
	CONSTRAINT crm_quotation_sends_status_check CHECK (status IN ('sent', 'failed')),
	CONSTRAINT fk_crm_quotation_sends_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_quotation_sends_quotation FOREIGN KEY (organization_id, quotation_id)
		REFERENCES crm_quotations(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_quotation_sends_user FOREIGN KEY (sent_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_crm_quotation_sends_quotation
	ON crm_quotation_sends(organization_id, quotation_id, sent_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_quotation_sends_client_request
	ON crm_quotation_sends(organization_id, quotation_id, client_request_id) WHERE client_request_id IS NOT NULL;
SELECT apply_organization_rls('crm_quotation_sends'::regclass);
