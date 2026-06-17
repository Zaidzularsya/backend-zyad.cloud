CREATE TABLE IF NOT EXISTS landing_media_assets (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	storage_key varchar(255) NOT NULL,
	filename varchar(255) NOT NULL,
	mime_type varchar(100) NOT NULL,
	size_bytes bigint NOT NULL,
	width integer,
	height integer,
	duration_seconds integer,
	alt_text varchar(255),
	processing_status varchar(30) NOT NULL DEFAULT 'completed',
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_media_assets_size_bytes_check CHECK (size_bytes > 0),
	CONSTRAINT landing_media_assets_processing_status_check CHECK (
		processing_status IN ('pending', 'completed', 'failed')
	),
	CONSTRAINT landing_media_assets_filename_not_blank_check CHECK (char_length(btrim(filename)) > 0),
	CONSTRAINT landing_media_assets_storage_key_not_blank_check CHECK (char_length(btrim(storage_key)) > 0),
	CONSTRAINT fk_landing_media_assets_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_media_assets_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_media_assets_org_key_active_unique
	ON landing_media_assets(organization_id, storage_key)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_media_assets_deleted_at
	ON landing_media_assets(deleted_at);

SELECT apply_organization_rls('landing_media_assets'::regclass);
