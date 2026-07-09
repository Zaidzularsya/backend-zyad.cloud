-- Recreate old billing_plans / billing_plan_prices / billing_features / billing_plan_entitlements
-- exactly as migration 000050_create_billing_plan_catalog.up.sql

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

-- Recreate old billing_subscriptions exactly as migration 000051_create_billing_subscriptions.up.sql

CREATE TABLE IF NOT EXISTS billing_subscriptions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	plan_id uuid NOT NULL,
	status varchar(30) NOT NULL,
	billing_interval varchar(30) NOT NULL,
	current_period_start timestamp without time zone,
	current_period_end timestamp without time zone,
	trial_start timestamp without time zone,
	trial_end timestamp without time zone,
	cancel_at_period_end boolean NOT NULL DEFAULT false,
	canceled_at timestamp without time zone,
	suspended_at timestamp without time zone,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_subscriptions_identity_unique UNIQUE (id, organization_id),
	CONSTRAINT billing_subscriptions_status_check CHECK (
		status IN (
			'trialing',
			'active',
			'past_due',
			'grace_period',
			'suspended',
			'canceled',
			'expired'
		)
	),
	CONSTRAINT billing_subscriptions_interval_check CHECK (
		billing_interval IN ('monthly', 'yearly', 'custom')
	),
	CONSTRAINT billing_subscriptions_period_check CHECK (
		current_period_start IS NULL
		OR current_period_end IS NULL
		OR current_period_end > current_period_start
	),
	CONSTRAINT billing_subscriptions_trial_period_check CHECK (
		trial_start IS NULL
		OR trial_end IS NULL
		OR trial_end > trial_start
	),
	CONSTRAINT billing_subscriptions_cancel_check CHECK (
		status <> 'canceled'
		OR canceled_at IS NOT NULL
	),
	CONSTRAINT billing_subscriptions_suspend_check CHECK (
		status <> 'suspended'
		OR suspended_at IS NOT NULL
	),
	CONSTRAINT billing_subscriptions_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_billing_subscriptions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_billing_subscriptions_plan_id
		FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_subscriptions_one_usable_per_org
	ON billing_subscriptions(organization_id)
	WHERE status IN ('trialing', 'active', 'past_due', 'grace_period');

CREATE INDEX IF NOT EXISTS idx_billing_subscriptions_org_status
	ON billing_subscriptions(organization_id, status);

CREATE INDEX IF NOT EXISTS idx_billing_subscriptions_plan
	ON billing_subscriptions(plan_id);

CREATE INDEX IF NOT EXISTS idx_billing_subscriptions_period_end
	ON billing_subscriptions(current_period_end)
	WHERE current_period_end IS NOT NULL;

-- Recreate old billing_subscription_events exactly as migration 000053_create_billing_events.up.sql (subscription events part)

CREATE TABLE IF NOT EXISTS billing_subscription_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	subscription_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	event_type varchar(80) NOT NULL,
	old_status varchar(30),
	new_status varchar(30),
	actor_user_id uuid,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_subscription_events_type_check CHECK (
		event_type = lower(event_type)
		AND event_type ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT billing_subscription_events_old_status_check CHECK (
		old_status IS NULL
		OR old_status IN (
			'trialing',
			'active',
			'past_due',
			'grace_period',
			'suspended',
			'canceled',
			'expired'
		)
	),
	CONSTRAINT billing_subscription_events_new_status_check CHECK (
		new_status IS NULL
		OR new_status IN (
			'trialing',
			'active',
			'past_due',
			'grace_period',
			'suspended',
			'canceled',
			'expired'
		)
	),
	CONSTRAINT billing_subscription_events_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_billing_subscription_events_subscription_org
		FOREIGN KEY (subscription_id, organization_id)
		REFERENCES billing_subscriptions(id, organization_id)
		ON DELETE CASCADE,
	CONSTRAINT fk_billing_subscription_events_actor_user_id
		FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_billing_subscription_events_subscription_created
	ON billing_subscription_events(subscription_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_billing_subscription_events_org_created
	ON billing_subscription_events(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_billing_subscription_events_type_created
	ON billing_subscription_events(event_type, created_at DESC);

-- Repoint billing_invoices' subscription FK back to the recreated billing_subscriptions table.
ALTER TABLE billing_invoices
	DROP CONSTRAINT IF EXISTS fk_billing_invoices_subscription_org;

ALTER TABLE billing_invoices
	ADD CONSTRAINT fk_billing_invoices_subscription_org
	FOREIGN KEY (subscription_id, organization_id)
	REFERENCES billing_subscriptions(id, organization_id)
	ON DELETE RESTRICT;
