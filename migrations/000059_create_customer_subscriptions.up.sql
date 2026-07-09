CREATE TABLE IF NOT EXISTS customer_subscriptions (
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
	CONSTRAINT customer_subscriptions_identity_unique UNIQUE (id, organization_id),
	CONSTRAINT customer_subscriptions_status_check CHECK (
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
	CONSTRAINT customer_subscriptions_interval_check CHECK (
		billing_interval IN ('monthly', 'yearly', 'custom')
	),
	CONSTRAINT customer_subscriptions_period_check CHECK (
		current_period_start IS NULL
		OR current_period_end IS NULL
		OR current_period_end > current_period_start
	),
	CONSTRAINT customer_subscriptions_trial_period_check CHECK (
		trial_start IS NULL
		OR trial_end IS NULL
		OR trial_end > trial_start
	),
	CONSTRAINT customer_subscriptions_cancel_check CHECK (
		status <> 'canceled'
		OR canceled_at IS NOT NULL
	),
	CONSTRAINT customer_subscriptions_suspend_check CHECK (
		status <> 'suspended'
		OR suspended_at IS NOT NULL
	),
	CONSTRAINT customer_subscriptions_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_customer_subscriptions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_customer_subscriptions_plan_id
		FOREIGN KEY (plan_id) REFERENCES product_plans(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_subscriptions_one_usable_per_org
	ON customer_subscriptions(organization_id)
	WHERE status IN ('trialing', 'active', 'past_due', 'grace_period');

CREATE INDEX IF NOT EXISTS idx_customer_subscriptions_org_status
	ON customer_subscriptions(organization_id, status);

CREATE INDEX IF NOT EXISTS idx_customer_subscriptions_plan
	ON customer_subscriptions(plan_id);

CREATE INDEX IF NOT EXISTS idx_customer_subscriptions_period_end
	ON customer_subscriptions(current_period_end)
	WHERE current_period_end IS NOT NULL;

CREATE TABLE IF NOT EXISTS subscription_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	subscription_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	event_type varchar(80) NOT NULL,
	old_status varchar(30),
	new_status varchar(30),
	actor_user_id uuid,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT subscription_events_type_check CHECK (
		event_type = lower(event_type)
		AND event_type ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT subscription_events_old_status_check CHECK (
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
	CONSTRAINT subscription_events_new_status_check CHECK (
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
	CONSTRAINT subscription_events_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_subscription_events_subscription_org
		FOREIGN KEY (subscription_id, organization_id)
		REFERENCES customer_subscriptions(id, organization_id)
		ON DELETE CASCADE,
	CONSTRAINT fk_subscription_events_actor_user_id
		FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_subscription_events_subscription_created
	ON subscription_events(subscription_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_subscription_events_org_created
	ON subscription_events(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_subscription_events_type_created
	ON subscription_events(event_type, created_at DESC);
