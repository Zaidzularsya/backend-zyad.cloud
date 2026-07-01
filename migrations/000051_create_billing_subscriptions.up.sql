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
