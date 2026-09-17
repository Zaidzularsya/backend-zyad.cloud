CREATE TABLE IF NOT EXISTS asset_objects (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	storage_key varchar(255) NOT NULL,
	filename varchar(255) NOT NULL,
	mime_type varchar(100) NOT NULL,
	size_bytes bigint NOT NULL,
	class varchar(20) NOT NULL DEFAULT 'private',
	label varchar(255),
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT asset_objects_size_bytes_check CHECK (size_bytes > 0),
	CONSTRAINT asset_objects_class_check CHECK (class IN ('public', 'private')),
	CONSTRAINT asset_objects_filename_not_blank_check CHECK (char_length(btrim(filename)) > 0),
	CONSTRAINT asset_objects_storage_key_not_blank_check CHECK (char_length(btrim(storage_key)) > 0),
	CONSTRAINT fk_asset_objects_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_asset_objects_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_objects_org_key_active_unique
	ON asset_objects(organization_id, storage_key)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_asset_objects_deleted_at
	ON asset_objects(deleted_at);

-- apply_organization_rls (migration 000019) generic, tidak khusus modul landing —
-- dipakai di sini juga karena tabel ini bisa menyimpan dokumen sensitif (mis. KTP).
SELECT apply_organization_rls('asset_objects'::regclass);
