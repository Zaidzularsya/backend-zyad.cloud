CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS organizations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	type varchar(30) NOT NULL,
	slug varchar(100) NOT NULL,
	name varchar(200) NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'pending',
	timezone varchar(100) NOT NULL DEFAULT 'Asia/Jakarta',
	locale varchar(20) NOT NULL DEFAULT 'id-ID',
	region varchar(100),
	data_placement varchar(30) NOT NULL DEFAULT 'shared',
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT organizations_type_check CHECK (
		type IN ('platform', 'customer')
	),
	CONSTRAINT organizations_status_check CHECK (
		status IN (
			'pending',
			'active',
			'suspended',
			'disabled',
			'archived',
			'provisioning',
			'provisioning_failed'
		)
	),
	CONSTRAINT organizations_data_placement_check CHECK (
		data_placement IN ('shared', 'dedicated')
	),
	CONSTRAINT organizations_slug_format_check CHECK (
		slug = lower(slug)
		AND slug ~ '^[a-z0-9]([a-z0-9-]{0,98}[a-z0-9])?$'
	),
	CONSTRAINT organizations_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT organizations_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_slug_active_unique
	ON organizations(lower(slug))
	WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_single_platform
	ON organizations(type)
	WHERE type = 'platform';

CREATE INDEX IF NOT EXISTS idx_organizations_type_status
	ON organizations(type, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organizations_data_placement
	ON organizations(data_placement)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organizations_deleted_at
	ON organizations(deleted_at);

CREATE OR REPLACE FUNCTION prevent_platform_organization_removal()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		IF OLD.type = 'platform' THEN
			RAISE EXCEPTION 'platform organization cannot be deleted';
		END IF;
		RETURN OLD;
	END IF;

	IF OLD.type = 'platform' THEN
		IF NEW.type <> OLD.type THEN
			RAISE EXCEPTION 'platform organization type cannot be changed';
		END IF;
		IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
			RAISE EXCEPTION 'platform organization cannot be archived by soft delete';
		END IF;
	END IF;

	RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_prevent_platform_organization_removal ON organizations;

CREATE TRIGGER trg_prevent_platform_organization_removal
	BEFORE UPDATE OR DELETE ON organizations
	FOR EACH ROW
	EXECUTE FUNCTION prevent_platform_organization_removal();
