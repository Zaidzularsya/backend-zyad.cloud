-- Router Mikrotik milik tenant (Fase 1 isolir PPPoE). Password disimpan terenkripsi
-- (password_encrypted); tls_fingerprint adalah SHA-256 hex sertifikat yang dipin.
CREATE TABLE IF NOT EXISTS mikrotik_routers (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	host varchar(255) NOT NULL,
	port integer NOT NULL,
	api_type varchar(10) NOT NULL,
	use_tls boolean NOT NULL,
	tls_fingerprint varchar(64),
	username varchar(100) NOT NULL,
	password_encrypted text NOT NULL,
	isolir_ready boolean NOT NULL DEFAULT false,
	isolir_ready_at timestamp without time zone,
	isolir_profile_name varchar(100),
	isolir_redirect_url text,
	last_check_at timestamp without time zone,
	last_check_status varchar(20),
	routeros_version varchar(50),
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT mikrotik_routers_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT mikrotik_routers_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT mikrotik_routers_port_check CHECK (port BETWEEN 1 AND 65535),
	CONSTRAINT mikrotik_routers_api_type_check CHECK (api_type IN ('rest', 'binary')),
	CONSTRAINT mikrotik_routers_last_check_status_check CHECK (
		last_check_status IS NULL OR last_check_status IN ('ok', 'unreachable', 'auth_failed', 'tls_untrusted')
	),
	CONSTRAINT fk_mikrotik_routers_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_routers_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_mikrotik_routers_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mikrotik_routers_org_name_unique
	ON mikrotik_routers(organization_id, lower(name))
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mikrotik_routers_deleted_at
	ON mikrotik_routers(deleted_at);

SELECT apply_organization_rls('mikrotik_routers'::regclass);

-- Registers the mikrotik.enabled feature key (boolean). Plans were retired (000154), so
-- there are no plan entitlement rows to seed: a tenant has the feature only when a
-- product/contract grants it, i.e. it is OFF by default for every existing tenant.
INSERT INTO product_features (feature_key, module, name, description, value_type, unit, reset_strategy, is_active, created_at, updated_at)
VALUES
	('mikrotik.enabled', 'mikrotik', 'Mikrotik router management', 'Register Mikrotik routers and manage PPPoE isolir', 'boolean', NULL, 'never', true, now(), now())
ON CONFLICT (feature_key)
DO UPDATE SET module = EXCLUDED.module, name = EXCLUDED.name, description = EXCLUDED.description,
	value_type = EXCLUDED.value_type, unit = EXCLUDED.unit, reset_strategy = EXCLUDED.reset_strategy,
	is_active = EXCLUDED.is_active, updated_at = now();
