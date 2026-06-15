CREATE TABLE IF NOT EXISTS organization_entitlements (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	feature_key varchar(150) NOT NULL,
	source varchar(30) NOT NULL,
	source_reference varchar(150),
	status varchar(30) NOT NULL DEFAULT 'active',
	limits jsonb NOT NULL DEFAULT '{}'::jsonb,
	version bigint NOT NULL DEFAULT 1,
	effective_from timestamp without time zone NOT NULL DEFAULT now(),
	effective_until timestamp without time zone,
	reason text,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT organization_entitlements_source_check CHECK (
		source IN ('plan', 'addon', 'trial', 'platform_override')
	),
	CONSTRAINT organization_entitlements_status_check CHECK (
		status IN ('active', 'inactive', 'expired', 'revoked')
	),
	CONSTRAINT organization_entitlements_feature_key_check CHECK (
		feature_key = lower(feature_key)
		AND feature_key ~ '^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*)+$'
	),
	CONSTRAINT organization_entitlements_limits_object_check CHECK (
		jsonb_typeof(limits) = 'object'
	),
	CONSTRAINT organization_entitlements_version_check CHECK (version > 0),
	CONSTRAINT organization_entitlements_window_check CHECK (
		effective_until IS NULL OR effective_until > effective_from
	),
	CONSTRAINT organization_entitlements_override_audit_check CHECK (
		source <> 'platform_override'
		OR (
			effective_until IS NOT NULL
			AND created_by IS NOT NULL
			AND char_length(btrim(COALESCE(reason, ''))) > 0
		)
	),
	CONSTRAINT fk_organization_entitlements_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_organization_entitlements_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT,
	CONSTRAINT fk_organization_entitlements_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_organization_entitlements_effective_lookup
	ON organization_entitlements(
		organization_id,
		feature_key,
		status,
		effective_from,
		effective_until
	);

CREATE INDEX IF NOT EXISTS idx_organization_entitlements_source_reference
	ON organization_entitlements(source, source_reference)
	WHERE source_reference IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_organization_entitlements_version
	ON organization_entitlements(organization_id, version);

CREATE TABLE IF NOT EXISTS organization_usage_counters (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	feature_key varchar(150) NOT NULL,
	metric_key varchar(150) NOT NULL,
	period_start timestamp without time zone NOT NULL,
	period_end timestamp without time zone NOT NULL,
	usage_value bigint NOT NULL DEFAULT 0,
	version bigint NOT NULL DEFAULT 1,
	last_recorded_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT organization_usage_counters_feature_key_check CHECK (
		feature_key = lower(feature_key)
		AND feature_key ~ '^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*)+$'
	),
	CONSTRAINT organization_usage_counters_metric_key_check CHECK (
		metric_key = lower(metric_key)
		AND metric_key ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT organization_usage_counters_period_check CHECK (
		period_end > period_start
	),
	CONSTRAINT organization_usage_counters_usage_value_check CHECK (
		usage_value >= 0
	),
	CONSTRAINT organization_usage_counters_version_check CHECK (version > 0),
	CONSTRAINT organization_usage_counters_period_unique UNIQUE (
		organization_id,
		feature_key,
		metric_key,
		period_start,
		period_end
	),
	CONSTRAINT fk_organization_usage_counters_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_organization_usage_counters_lookup
	ON organization_usage_counters(
		organization_id,
		feature_key,
		metric_key,
		period_start,
		period_end
	);

CREATE OR REPLACE FUNCTION increment_entitlement_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF NEW.feature_key IS DISTINCT FROM OLD.feature_key
		OR NEW.source IS DISTINCT FROM OLD.source
		OR NEW.source_reference IS DISTINCT FROM OLD.source_reference
		OR NEW.status IS DISTINCT FROM OLD.status
		OR NEW.limits IS DISTINCT FROM OLD.limits
		OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
		OR NEW.effective_until IS DISTINCT FROM OLD.effective_until THEN
		NEW.version := GREATEST(NEW.version, OLD.version + 1);
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_increment_entitlement_version
	BEFORE UPDATE ON organization_entitlements
	FOR EACH ROW
	EXECUTE FUNCTION increment_entitlement_version();

CREATE OR REPLACE FUNCTION increment_usage_counter_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF NEW.usage_value IS DISTINCT FROM OLD.usage_value
		OR NEW.period_start IS DISTINCT FROM OLD.period_start
		OR NEW.period_end IS DISTINCT FROM OLD.period_end THEN
		NEW.version := GREATEST(NEW.version, OLD.version + 1);
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trg_increment_usage_counter_version
	BEFORE UPDATE ON organization_usage_counters
	FOR EACH ROW
	EXECUTE FUNCTION increment_usage_counter_version();
