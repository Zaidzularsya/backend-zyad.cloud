CREATE TABLE IF NOT EXISTS reserved_subdomains (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	label varchar(63) NOT NULL,
	reason varchar(255),
	is_system boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT reserved_subdomains_label_format_check CHECK (
		label = lower(label)
		AND label ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
	),
	CONSTRAINT reserved_subdomains_label_unique UNIQUE (label)
);

CREATE OR REPLACE FUNCTION canonical_hostname_is_valid(hostname text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
	SELECT
		hostname IS NOT NULL
		AND char_length(hostname) BETWEEN 3 AND 253
		AND hostname = lower(hostname)
		AND hostname !~ '[:/]'
		AND hostname !~ '[[:space:]]'
		AND hostname !~ '[.]$'
		AND position('.' IN hostname) > 0
		AND NOT EXISTS (
			SELECT 1
			FROM regexp_split_to_table(hostname, '[.]') AS labels(label)
			WHERE label !~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'
		);
$$;

CREATE TABLE IF NOT EXISTS organization_domains (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	type varchar(30) NOT NULL,
	canonical_host varchar(253) NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'pending',
	is_primary boolean NOT NULL DEFAULT false,
	verification_challenge_hash text,
	verification_attempts integer NOT NULL DEFAULT 0,
	last_verification_at timestamp without time zone,
	verified_at timestamp without time zone,
	verification_error text,
	ssl_status varchar(30) NOT NULL DEFAULT 'pending',
	ssl_error text,
	ssl_expires_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT organization_domains_type_check CHECK (
		type IN ('platform', 'subdomain', 'custom')
	),
	CONSTRAINT organization_domains_status_check CHECK (
		status IN ('pending', 'verified', 'active', 'failed', 'disabled')
	),
	CONSTRAINT organization_domains_ssl_status_check CHECK (
		ssl_status IN ('pending', 'provisioning', 'active', 'failed', 'not_required')
	),
	CONSTRAINT organization_domains_host_format_check CHECK (
		canonical_hostname_is_valid(canonical_host)
	),
	CONSTRAINT organization_domains_verification_attempts_check CHECK (
		verification_attempts >= 0
	),
	CONSTRAINT organization_domains_challenge_hash_check CHECK (
		verification_challenge_hash IS NULL
		OR char_length(btrim(verification_challenge_hash)) >= 32
	),
	CONSTRAINT organization_domains_verified_at_check CHECK (
		status NOT IN ('verified', 'active')
		OR verified_at IS NOT NULL
	),
	CONSTRAINT fk_organization_domains_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_canonical_host_unique
	ON organization_domains(lower(canonical_host));

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_domains_primary_active_unique
	ON organization_domains(organization_id, type)
	WHERE is_primary = true
		AND status = 'active'
		AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organization_domains_organization_status
	ON organization_domains(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organization_domains_public_resolution
	ON organization_domains(canonical_host, organization_id)
	WHERE status = 'active' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_organization_domains_ssl_status
	ON organization_domains(ssl_status)
	WHERE deleted_at IS NULL;

CREATE OR REPLACE FUNCTION reject_reserved_organization_subdomain()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	subdomain_label text;
BEGIN
	IF NEW.type <> 'subdomain' OR NEW.deleted_at IS NOT NULL THEN
		RETURN NEW;
	END IF;

	subdomain_label := split_part(NEW.canonical_host, '.', 1);
	IF EXISTS (
		SELECT 1
		FROM reserved_subdomains
		WHERE label = subdomain_label
	) THEN
		RAISE EXCEPTION 'subdomain label "%" is reserved', subdomain_label;
	END IF;

	RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION reject_claimed_reserved_subdomain()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM organization_domains
		WHERE type = 'subdomain'
			AND split_part(canonical_host, '.', 1) = NEW.label
			AND deleted_at IS NULL
	) THEN
		RAISE EXCEPTION 'subdomain label "%" is already claimed', NEW.label;
	END IF;

	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_reject_reserved_organization_subdomain
	BEFORE INSERT OR UPDATE OF type, canonical_host, deleted_at
	ON organization_domains
	FOR EACH ROW
	EXECUTE FUNCTION reject_reserved_organization_subdomain();

CREATE TRIGGER trg_reject_claimed_reserved_subdomain
	BEFORE INSERT OR UPDATE OF label
	ON reserved_subdomains
	FOR EACH ROW
	EXECUTE FUNCTION reject_claimed_reserved_subdomain();
