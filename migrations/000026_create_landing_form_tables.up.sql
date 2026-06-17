CREATE TABLE IF NOT EXISTS landing_forms (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	key varchar(120) NOT NULL,
	description text,
	submit_label varchar(100) NOT NULL DEFAULT 'Submit',
	success_message text,
	redirect_url text,
	is_active boolean NOT NULL DEFAULT true,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_forms_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT landing_forms_key_format_check CHECK (
		key = lower(key)
		AND key ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT landing_forms_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT fk_landing_forms_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_forms_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_forms_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_forms_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_forms_page_key_active_unique
	ON landing_forms(organization_id, landing_page_id, lower(key))
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_forms_deleted_at
	ON landing_forms(deleted_at);


CREATE TABLE IF NOT EXISTS landing_form_fields (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	form_id uuid NOT NULL,
	field_key varchar(120) NOT NULL,
	field_type varchar(50) NOT NULL,
	label varchar(255) NOT NULL,
	placeholder varchar(255),
	options jsonb NOT NULL DEFAULT '[]'::jsonb,
	validation jsonb NOT NULL DEFAULT '{}'::jsonb,
	is_required boolean NOT NULL DEFAULT false,
	sort_order integer NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_form_fields_field_key_format_check CHECK (
		field_key = lower(field_key)
		AND field_key ~ '^[a-z0-9]([a-z0-9_]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT landing_form_fields_field_type_check CHECK (
		field_type IN (
			'text',
			'textarea',
			'email',
			'phone',
			'number',
			'select',
			'radio',
			'checkbox',
			'file',
			'date',
			'hidden'
		)
	),
	CONSTRAINT landing_form_fields_label_not_blank_check CHECK (
		char_length(btrim(label)) > 0
	),
	CONSTRAINT landing_form_fields_sort_order_check CHECK (
		sort_order >= 0
	),
	CONSTRAINT landing_form_fields_options_array_check CHECK (
		jsonb_typeof(options) = 'array'
	),
	CONSTRAINT landing_form_fields_validation_object_check CHECK (
		jsonb_typeof(validation) = 'object'
	),
	CONSTRAINT fk_landing_form_fields_form
		FOREIGN KEY (organization_id, form_id)
		REFERENCES landing_forms(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_form_fields_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_form_fields_form_key_unique
	ON landing_form_fields(organization_id, form_id, lower(field_key));

CREATE INDEX IF NOT EXISTS idx_landing_form_fields_form_sort
	ON landing_form_fields(organization_id, form_id, sort_order);


CREATE TABLE IF NOT EXISTS landing_submissions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	form_id uuid NOT NULL,
	reference varchar(100) NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'new',
	submitted_data jsonb NOT NULL DEFAULT '{}'::jsonb,
	source_url text,
	referrer text,
	utm_source varchar(100),
	utm_medium varchar(100),
	utm_campaign varchar(100),
	utm_term varchar(100),
	utm_content varchar(100),
	ip_address_hash varchar(64),
	user_agent text,
	idempotency_key varchar(120),
	submitted_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_submissions_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT landing_submissions_status_check CHECK (
		status IN (
			'new',
			'contacted',
			'qualified',
			'converted',
			'rejected',
			'spam',
			'archived'
		)
	),
	CONSTRAINT landing_submissions_submitted_data_object_check CHECK (
		jsonb_typeof(submitted_data) = 'object'
	),
	CONSTRAINT fk_landing_submissions_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_submissions_form
		FOREIGN KEY (organization_id, form_id)
		REFERENCES landing_forms(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_submissions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_submissions_organization_idempotency
	ON landing_submissions(organization_id, idempotency_key)
	WHERE idempotency_key IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_submissions_deleted_at
	ON landing_submissions(deleted_at);

CREATE INDEX IF NOT EXISTS idx_landing_submissions_org_page_form
	ON landing_submissions(organization_id, landing_page_id, form_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_submissions_submitted_at
	ON landing_submissions(organization_id, submitted_at)
	WHERE deleted_at IS NULL;


CREATE TABLE IF NOT EXISTS landing_submission_notes (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	submission_id uuid NOT NULL,
	note text NOT NULL,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_submission_notes_note_not_blank_check CHECK (
		char_length(btrim(note)) > 0
	),
	CONSTRAINT fk_landing_submission_notes_submission
		FOREIGN KEY (organization_id, submission_id)
		REFERENCES landing_submissions(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_submission_notes_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_submission_notes_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_landing_submission_notes_submission_id
	ON landing_submission_notes(organization_id, submission_id);

SELECT apply_organization_rls('landing_forms'::regclass);
SELECT apply_organization_rls('landing_form_fields'::regclass);
SELECT apply_organization_rls('landing_submissions'::regclass);
SELECT apply_organization_rls('landing_submission_notes'::regclass);
