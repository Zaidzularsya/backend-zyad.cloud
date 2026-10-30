ALTER TABLE landing_forms
	DROP COLUMN IF EXISTS whatsapp_message,
	DROP COLUMN IF EXISTS whatsapp_number,
	DROP COLUMN IF EXISTS after_submit;

ALTER TABLE landing_form_fields DROP COLUMN IF EXISTS default_value;
