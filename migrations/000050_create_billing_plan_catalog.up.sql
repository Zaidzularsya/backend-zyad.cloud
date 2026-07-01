CREATE TABLE IF NOT EXISTS billing_plans (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	code varchar(80) NOT NULL,
	name varchar(150) NOT NULL,
	description text,
	plan_type varchar(30) NOT NULL,
	is_public boolean NOT NULL DEFAULT true,
	is_active boolean NOT NULL DEFAULT true,
	sort_order integer NOT NULL DEFAULT 0,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT billing_plans_code_format_check CHECK (
		code = lower(code)
		AND code ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT billing_plans_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT billing_plans_plan_type_check CHECK (
		plan_type IN ('free', 'trial', 'paid', 'enterprise')
	),
	CONSTRAINT billing_plans_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_plans_code_active_unique
	ON billing_plans(code)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_billing_plans_public_active_sort
	ON billing_plans(is_public, is_active, sort_order)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_billing_plans_deleted_at
	ON billing_plans(deleted_at);

CREATE TABLE IF NOT EXISTS billing_plan_prices (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	plan_id uuid NOT NULL,
	billing_interval varchar(30) NOT NULL,
	currency varchar(3) NOT NULL DEFAULT 'IDR',
	amount numeric(14,2) NOT NULL DEFAULT 0,
	is_active boolean NOT NULL DEFAULT true,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT billing_plan_prices_interval_check CHECK (
		billing_interval IN ('monthly', 'yearly', 'one_time', 'custom')
	),
	CONSTRAINT billing_plan_prices_currency_check CHECK (
		currency = upper(currency)
		AND currency ~ '^[A-Z]{3}$'
	),
	CONSTRAINT billing_plan_prices_amount_check CHECK (amount >= 0),
	CONSTRAINT billing_plan_prices_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_billing_plan_prices_plan_id
		FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_plan_prices_active_unique
	ON billing_plan_prices(plan_id, billing_interval, currency)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_billing_plan_prices_plan_active
	ON billing_plan_prices(plan_id, is_active)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_billing_plan_prices_deleted_at
	ON billing_plan_prices(deleted_at);

CREATE TABLE IF NOT EXISTS billing_features (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	feature_key varchar(150) NOT NULL,
	module varchar(80) NOT NULL,
	name varchar(150) NOT NULL,
	description text,
	value_type varchar(30) NOT NULL,
	unit varchar(50),
	reset_strategy varchar(30) NOT NULL DEFAULT 'never',
	is_active boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_features_feature_key_unique UNIQUE (feature_key),
	CONSTRAINT billing_features_feature_key_check CHECK (
		feature_key = lower(feature_key)
		AND feature_key ~ '^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*)+$'
	),
	CONSTRAINT billing_features_module_check CHECK (
		module = lower(module)
		AND module ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT billing_features_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT billing_features_value_type_check CHECK (
		value_type IN ('boolean', 'integer', 'string', 'decimal')
	),
	CONSTRAINT billing_features_reset_strategy_check CHECK (
		reset_strategy IN ('never', 'monthly', 'yearly', 'custom')
	)
);

CREATE INDEX IF NOT EXISTS idx_billing_features_module_active
	ON billing_features(module, is_active);

CREATE TABLE IF NOT EXISTS billing_plan_entitlements (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	plan_id uuid NOT NULL,
	feature_id uuid NOT NULL,
	value_bool boolean,
	value_int bigint,
	value_decimal numeric(14,2),
	value_string text,
	limits jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_plan_entitlements_plan_feature_unique UNIQUE (plan_id, feature_id),
	CONSTRAINT billing_plan_entitlements_single_value_check CHECK (
		num_nonnulls(value_bool, value_int, value_decimal, value_string) = 1
	),
	CONSTRAINT billing_plan_entitlements_limits_object_check CHECK (
		jsonb_typeof(limits) = 'object'
	),
	CONSTRAINT fk_billing_plan_entitlements_plan_id
		FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE CASCADE,
	CONSTRAINT fk_billing_plan_entitlements_feature_id
		FOREIGN KEY (feature_id) REFERENCES billing_features(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_billing_plan_entitlements_feature
	ON billing_plan_entitlements(feature_id);
