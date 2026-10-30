-- Form Konsultasi fleksibel (S1): nilai default field tersembunyi, aksi setelah kirim,
-- dan tujuan WhatsApp per form. Kolom aditif; form lama tetap berfungsi.
ALTER TABLE landing_form_fields ADD COLUMN default_value text NULL;

ALTER TABLE landing_forms
	ADD COLUMN after_submit varchar(20) NOT NULL DEFAULT 'message'
		CHECK (after_submit IN ('message', 'redirect', 'whatsapp')),
	ADD COLUMN whatsapp_number varchar(20) NULL,
	ADD COLUMN whatsapp_message text NULL;

-- Form yang sudah punya redirect_url mempertahankan perilakunya.
UPDATE landing_forms SET after_submit = 'redirect'
 WHERE coalesce(trim(redirect_url), '') <> '';
