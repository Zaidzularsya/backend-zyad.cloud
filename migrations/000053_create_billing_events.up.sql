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

CREATE TABLE IF NOT EXISTS billing_payment_events (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	payment_id uuid,
	invoice_id uuid,
	provider varchar(30) NOT NULL,
	event_type varchar(80) NOT NULL,
	provider_event_id varchar(150),
	payload jsonb NOT NULL DEFAULT '{}'::jsonb,
	processed_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_payment_events_provider_check CHECK (
		provider IN ('manual', 'xendit', 'midtrans')
	),
	CONSTRAINT billing_payment_events_type_check CHECK (
		event_type = lower(event_type)
		AND event_type ~ '^[a-z][a-z0-9_]*$'
	),
	CONSTRAINT billing_payment_events_payload_object_check CHECK (
		jsonb_typeof(payload) = 'object'
	),
	CONSTRAINT fk_billing_payment_events_payment_id
		FOREIGN KEY (payment_id) REFERENCES billing_payments(id) ON DELETE SET NULL,
	CONSTRAINT fk_billing_payment_events_invoice_id
		FOREIGN KEY (invoice_id) REFERENCES billing_invoices(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_payment_events_provider_event_unique
	ON billing_payment_events(provider, provider_event_id)
	WHERE provider_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_billing_payment_events_invoice_created
	ON billing_payment_events(invoice_id, created_at DESC)
	WHERE invoice_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_billing_payment_events_payment_created
	ON billing_payment_events(payment_id, created_at DESC)
	WHERE payment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_billing_payment_events_provider_type_created
	ON billing_payment_events(provider, event_type, created_at DESC);
