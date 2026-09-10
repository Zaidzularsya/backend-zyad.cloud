BEGIN;

DROP TABLE IF EXISTS landing_page_documents CASCADE;

ALTER TABLE landing_pages DROP CONSTRAINT IF EXISTS landing_pages_builder_check;
ALTER TABLE landing_pages DROP COLUMN IF EXISTS builder;

COMMIT;
