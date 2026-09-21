CREATE TABLE IF NOT EXISTS crm_document_counters (
	organization_id uuid NOT NULL,
	document_type   varchar(20) NOT NULL,
	year            smallint NOT NULL,
	last_number     integer NOT NULL DEFAULT 0,
	created_at      timestamp without time zone NOT NULL DEFAULT now(),
	updated_at      timestamp without time zone NOT NULL DEFAULT now(),
	PRIMARY KEY (organization_id, document_type, year),
	CONSTRAINT fk_crm_document_counters_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT crm_document_counters_document_type_check CHECK (
		document_type IN ('quotation', 'invoice')
	)
);

SELECT apply_organization_rls('crm_document_counters'::regclass);
