-- Normalized phone for matching inbound WhatsApp numbers to CRM leads/contacts.
-- Rules (mirrored by internal/shared/phone.NormalizeID, kept in sync by an
-- integration test): keep digits only; "00" international prefix is dropped;
-- a leading "0" becomes "62"; a leading "8" (Indonesian mobile without prefix)
-- becomes "628"; the result must be 8-15 digits, otherwise NULL.
CREATE OR REPLACE FUNCTION normalize_phone_id(raw text)
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
	SELECT CASE WHEN char_length(normalized.value) BETWEEN 8 AND 15 THEN normalized.value END
	FROM (
		SELECT CASE
			WHEN digits.value LIKE '00%' THEN substr(digits.value, 3)
			WHEN digits.value LIKE '0%' THEN '62' || substr(digits.value, 2)
			WHEN digits.value LIKE '8%' THEN '62' || digits.value
			ELSE digits.value
		END AS value
		FROM (SELECT regexp_replace(COALESCE(raw, ''), '[^0-9]', '', 'g') AS value) AS digits
	) AS normalized
$$;

-- Generated STORED columns rewrite the table once; crm_leads/crm_contacts are
-- small enough for this to be acceptable. The value is derived from phone, so
-- it stays correct for every write path (API, lead convert, future imports).
ALTER TABLE crm_leads
	ADD COLUMN IF NOT EXISTS phone_normalized varchar(20)
	GENERATED ALWAYS AS (normalize_phone_id(phone)) STORED;

ALTER TABLE crm_contacts
	ADD COLUMN IF NOT EXISTS phone_normalized varchar(20)
	GENERATED ALWAYS AS (normalize_phone_id(phone)) STORED;

CREATE INDEX IF NOT EXISTS idx_crm_leads_organization_phone_normalized
	ON crm_leads(organization_id, phone_normalized)
	WHERE deleted_at IS NULL AND phone_normalized IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_crm_contacts_organization_phone_normalized
	ON crm_contacts(organization_id, phone_normalized)
	WHERE deleted_at IS NULL AND phone_normalized IS NOT NULL;
