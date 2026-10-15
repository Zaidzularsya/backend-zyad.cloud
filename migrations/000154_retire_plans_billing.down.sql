-- R4-S5 rollback: membuat ulang STRUKTUR KOSONG plan/subscription/billing platform lama beserta permission-nya.
-- Data TIDAK ikut kembali; pulihkan dari arsip r4-s5-archive-<tanggal>.sql (pg_dump sebelum migration up).
-- DDL di bawah adalah skema hasil seluruh migration sebelumnya (000050..000153), dikeluarkan dengan pg_dump -s.

CREATE TABLE billing_invoice_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    invoice_id uuid NOT NULL,
    item_type character varying(30) NOT NULL,
    description text NOT NULL,
    quantity numeric(14,2) DEFAULT 1 NOT NULL,
    unit_amount numeric(14,2) DEFAULT 0 NOT NULL,
    total_amount numeric(14,2) DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_invoice_items_amount_check CHECK (((quantity > (0)::numeric) AND (unit_amount >= (0)::numeric) AND (total_amount >= (0)::numeric))),
    CONSTRAINT billing_invoice_items_description_not_blank_check CHECK ((char_length(btrim(description)) > 0)),
    CONSTRAINT billing_invoice_items_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT billing_invoice_items_type_check CHECK (((item_type)::text = ANY ((ARRAY['subscription'::character varying, 'addon'::character varying, 'adjustment'::character varying, 'tax'::character varying, 'discount'::character varying])::text[])))
);

CREATE TABLE billing_invoices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    subscription_id uuid,
    invoice_number character varying(80) NOT NULL,
    status character varying(30) NOT NULL,
    currency character varying(3) DEFAULT 'IDR'::character varying NOT NULL,
    subtotal_amount numeric(14,2) DEFAULT 0 NOT NULL,
    discount_amount numeric(14,2) DEFAULT 0 NOT NULL,
    tax_amount numeric(14,2) DEFAULT 0 NOT NULL,
    total_amount numeric(14,2) DEFAULT 0 NOT NULL,
    due_date timestamp without time zone,
    paid_at timestamp without time zone,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_invoices_amount_check CHECK (((subtotal_amount >= (0)::numeric) AND (discount_amount >= (0)::numeric) AND (tax_amount >= (0)::numeric) AND (total_amount >= (0)::numeric))),
    CONSTRAINT billing_invoices_currency_check CHECK ((((currency)::text = upper((currency)::text)) AND ((currency)::text ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT billing_invoices_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT billing_invoices_paid_check CHECK ((((status)::text <> 'paid'::text) OR (paid_at IS NOT NULL))),
    CONSTRAINT billing_invoices_status_check CHECK (((status)::text = ANY ((ARRAY['draft'::character varying, 'open'::character varying, 'paid'::character varying, 'void'::character varying, 'expired'::character varying, 'failed'::character varying])::text[])))
);

CREATE TABLE billing_payment_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    payment_id uuid,
    invoice_id uuid,
    provider character varying(30) NOT NULL,
    event_type character varying(80) NOT NULL,
    provider_event_id character varying(150),
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    processed_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_payment_events_payload_object_check CHECK ((jsonb_typeof(payload) = 'object'::text)),
    CONSTRAINT billing_payment_events_provider_check CHECK (((provider)::text = ANY ((ARRAY['manual'::character varying, 'xendit'::character varying, 'midtrans'::character varying, 'doku'::character varying])::text[]))),
    CONSTRAINT billing_payment_events_type_check CHECK ((((event_type)::text = lower((event_type)::text)) AND ((event_type)::text ~ '^[a-z][a-z0-9_]*$'::text)))
);

CREATE TABLE billing_payments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    invoice_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    provider character varying(30) NOT NULL,
    provider_reference character varying(150),
    payment_method character varying(80),
    status character varying(30) NOT NULL,
    amount numeric(14,2) NOT NULL,
    currency character varying(3) DEFAULT 'IDR'::character varying NOT NULL,
    paid_at timestamp without time zone,
    raw_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_payments_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT billing_payments_currency_check CHECK ((((currency)::text = upper((currency)::text)) AND ((currency)::text ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT billing_payments_paid_check CHECK ((((status)::text <> 'paid'::text) OR (paid_at IS NOT NULL))),
    CONSTRAINT billing_payments_provider_check CHECK (((provider)::text = ANY ((ARRAY['manual'::character varying, 'xendit'::character varying, 'midtrans'::character varying, 'doku'::character varying])::text[]))),
    CONSTRAINT billing_payments_raw_payload_object_check CHECK ((jsonb_typeof(raw_payload) = 'object'::text)),
    CONSTRAINT billing_payments_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'paid'::character varying, 'failed'::character varying, 'expired'::character varying, 'refunded'::character varying])::text[])))
);

CREATE TABLE customer_subscriptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    status character varying(30) NOT NULL,
    billing_interval character varying(30) NOT NULL,
    current_period_start timestamp without time zone,
    current_period_end timestamp without time zone,
    trial_start timestamp without time zone,
    trial_end timestamp without time zone,
    cancel_at_period_end boolean DEFAULT false NOT NULL,
    canceled_at timestamp without time zone,
    suspended_at timestamp without time zone,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT customer_subscriptions_cancel_check CHECK ((((status)::text <> 'canceled'::text) OR (canceled_at IS NOT NULL))),
    CONSTRAINT customer_subscriptions_interval_check CHECK (((billing_interval)::text = ANY ((ARRAY['monthly'::character varying, 'yearly'::character varying, 'custom'::character varying])::text[]))),
    CONSTRAINT customer_subscriptions_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT customer_subscriptions_period_check CHECK (((current_period_start IS NULL) OR (current_period_end IS NULL) OR (current_period_end > current_period_start))),
    CONSTRAINT customer_subscriptions_status_check CHECK (((status)::text = ANY ((ARRAY['trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'grace_period'::character varying, 'suspended'::character varying, 'canceled'::character varying, 'expired'::character varying])::text[]))),
    CONSTRAINT customer_subscriptions_suspend_check CHECK ((((status)::text <> 'suspended'::text) OR (suspended_at IS NOT NULL))),
    CONSTRAINT customer_subscriptions_trial_period_check CHECK (((trial_start IS NULL) OR (trial_end IS NULL) OR (trial_end > trial_start)))
);

CREATE TABLE product_plan_entitlements (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    plan_id uuid NOT NULL,
    feature_id uuid NOT NULL,
    value_bool boolean,
    value_int bigint,
    value_decimal numeric(14,2),
    value_string text,
    limits jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT product_plan_entitlements_limits_object_check CHECK ((jsonb_typeof(limits) = 'object'::text)),
    CONSTRAINT product_plan_entitlements_single_value_check CHECK ((num_nonnulls(value_bool, value_int, value_decimal, value_string) = 1))
);

CREATE TABLE product_plan_prices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    plan_id uuid NOT NULL,
    billing_interval character varying(30) NOT NULL,
    currency character varying(3) DEFAULT 'IDR'::character varying NOT NULL,
    amount numeric(14,2) DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    CONSTRAINT product_plan_prices_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT product_plan_prices_currency_check CHECK ((((currency)::text = upper((currency)::text)) AND ((currency)::text ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT product_plan_prices_interval_check CHECK (((billing_interval)::text = ANY ((ARRAY['monthly'::character varying, 'yearly'::character varying, 'one_time'::character varying, 'custom'::character varying])::text[]))),
    CONSTRAINT product_plan_prices_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text))
);

CREATE TABLE product_plans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    code character varying(80) NOT NULL,
    name character varying(150) NOT NULL,
    description text,
    plan_type character varying(30) NOT NULL,
    is_public boolean DEFAULT true NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    CONSTRAINT product_plans_code_format_check CHECK ((((code)::text = lower((code)::text)) AND ((code)::text ~ '^[a-z][a-z0-9_]*$'::text))),
    CONSTRAINT product_plans_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT product_plans_name_not_blank_check CHECK ((char_length(btrim((name)::text)) > 0)),
    CONSTRAINT product_plans_plan_type_check CHECK (((plan_type)::text = ANY ((ARRAY['free'::character varying, 'trial'::character varying, 'paid'::character varying, 'enterprise'::character varying])::text[])))
);

CREATE TABLE subscription_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    subscription_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    event_type character varying(80) NOT NULL,
    old_status character varying(30),
    new_status character varying(30),
    actor_user_id uuid,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT subscription_events_metadata_object_check CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT subscription_events_new_status_check CHECK (((new_status IS NULL) OR ((new_status)::text = ANY ((ARRAY['trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'grace_period'::character varying, 'suspended'::character varying, 'canceled'::character varying, 'expired'::character varying])::text[])))),
    CONSTRAINT subscription_events_old_status_check CHECK (((old_status IS NULL) OR ((old_status)::text = ANY ((ARRAY['trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'grace_period'::character varying, 'suspended'::character varying, 'canceled'::character varying, 'expired'::character varying])::text[])))),
    CONSTRAINT subscription_events_type_check CHECK ((((event_type)::text = lower((event_type)::text)) AND ((event_type)::text ~ '^[a-z][a-z0-9_]*$'::text)))
);

ALTER TABLE billing_invoice_items
    ADD CONSTRAINT billing_invoice_items_pkey PRIMARY KEY (id);

ALTER TABLE billing_invoices
    ADD CONSTRAINT billing_invoices_identity_unique UNIQUE (id, organization_id);

ALTER TABLE billing_invoices
    ADD CONSTRAINT billing_invoices_number_unique UNIQUE (invoice_number);

ALTER TABLE billing_invoices
    ADD CONSTRAINT billing_invoices_pkey PRIMARY KEY (id);

ALTER TABLE billing_payment_events
    ADD CONSTRAINT billing_payment_events_pkey PRIMARY KEY (id);

ALTER TABLE billing_payments
    ADD CONSTRAINT billing_payments_pkey PRIMARY KEY (id);

ALTER TABLE customer_subscriptions
    ADD CONSTRAINT customer_subscriptions_identity_unique UNIQUE (id, organization_id);

ALTER TABLE customer_subscriptions
    ADD CONSTRAINT customer_subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE product_plan_entitlements
    ADD CONSTRAINT product_plan_entitlements_pkey PRIMARY KEY (id);

ALTER TABLE product_plan_entitlements
    ADD CONSTRAINT product_plan_entitlements_plan_feature_unique UNIQUE (plan_id, feature_id);

ALTER TABLE product_plan_prices
    ADD CONSTRAINT product_plan_prices_pkey PRIMARY KEY (id);

ALTER TABLE product_plans
    ADD CONSTRAINT product_plans_pkey PRIMARY KEY (id);

ALTER TABLE subscription_events
    ADD CONSTRAINT subscription_events_pkey PRIMARY KEY (id);

CREATE INDEX idx_billing_invoice_items_invoice ON billing_invoice_items USING btree (invoice_id);

CREATE INDEX idx_billing_invoices_due_date ON billing_invoices USING btree (due_date) WHERE (due_date IS NOT NULL);

CREATE INDEX idx_billing_invoices_org_status_created ON billing_invoices USING btree (organization_id, status, created_at DESC);

CREATE INDEX idx_billing_invoices_subscription ON billing_invoices USING btree (subscription_id) WHERE (subscription_id IS NOT NULL);

CREATE INDEX idx_billing_payment_events_invoice_created ON billing_payment_events USING btree (invoice_id, created_at DESC) WHERE (invoice_id IS NOT NULL);

CREATE INDEX idx_billing_payment_events_payment_created ON billing_payment_events USING btree (payment_id, created_at DESC) WHERE (payment_id IS NOT NULL);

CREATE UNIQUE INDEX idx_billing_payment_events_provider_event_unique ON billing_payment_events USING btree (provider, provider_event_id) WHERE (provider_event_id IS NOT NULL);

CREATE INDEX idx_billing_payment_events_provider_type_created ON billing_payment_events USING btree (provider, event_type, created_at DESC);

CREATE INDEX idx_billing_payments_invoice ON billing_payments USING btree (invoice_id);

CREATE INDEX idx_billing_payments_org_status_created ON billing_payments USING btree (organization_id, status, created_at DESC);

CREATE UNIQUE INDEX idx_billing_payments_provider_reference_unique ON billing_payments USING btree (provider, provider_reference) WHERE (provider_reference IS NOT NULL);

CREATE UNIQUE INDEX idx_customer_subscriptions_one_usable_per_org ON customer_subscriptions USING btree (organization_id) WHERE ((status)::text = ANY ((ARRAY['trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'grace_period'::character varying])::text[]));

CREATE INDEX idx_customer_subscriptions_org_status ON customer_subscriptions USING btree (organization_id, status);

CREATE INDEX idx_customer_subscriptions_period_end ON customer_subscriptions USING btree (current_period_end) WHERE (current_period_end IS NOT NULL);

CREATE INDEX idx_customer_subscriptions_plan ON customer_subscriptions USING btree (plan_id);

CREATE INDEX idx_product_plan_entitlements_feature ON product_plan_entitlements USING btree (feature_id);

CREATE UNIQUE INDEX idx_product_plan_prices_active_unique ON product_plan_prices USING btree (plan_id, billing_interval, currency) WHERE (deleted_at IS NULL);

CREATE INDEX idx_product_plan_prices_deleted_at ON product_plan_prices USING btree (deleted_at);

CREATE INDEX idx_product_plan_prices_plan_active ON product_plan_prices USING btree (plan_id, is_active) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_product_plans_code_active_unique ON product_plans USING btree (code) WHERE (deleted_at IS NULL);

CREATE INDEX idx_product_plans_deleted_at ON product_plans USING btree (deleted_at);

CREATE INDEX idx_product_plans_public_active_sort ON product_plans USING btree (is_public, is_active, sort_order) WHERE (deleted_at IS NULL);

CREATE INDEX idx_subscription_events_org_created ON subscription_events USING btree (organization_id, created_at DESC);

CREATE INDEX idx_subscription_events_subscription_created ON subscription_events USING btree (subscription_id, created_at DESC);

CREATE INDEX idx_subscription_events_type_created ON subscription_events USING btree (event_type, created_at DESC);

ALTER TABLE billing_invoice_items
    ADD CONSTRAINT fk_billing_invoice_items_invoice_id FOREIGN KEY (invoice_id) REFERENCES billing_invoices(id) ON DELETE CASCADE;

ALTER TABLE billing_invoices
    ADD CONSTRAINT fk_billing_invoices_organization_id FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

ALTER TABLE billing_invoices
    ADD CONSTRAINT fk_billing_invoices_subscription_org FOREIGN KEY (subscription_id, organization_id) REFERENCES customer_subscriptions(id, organization_id) ON DELETE RESTRICT;

ALTER TABLE billing_payment_events
    ADD CONSTRAINT fk_billing_payment_events_invoice_id FOREIGN KEY (invoice_id) REFERENCES billing_invoices(id) ON DELETE SET NULL;

ALTER TABLE billing_payment_events
    ADD CONSTRAINT fk_billing_payment_events_payment_id FOREIGN KEY (payment_id) REFERENCES billing_payments(id) ON DELETE SET NULL;

ALTER TABLE billing_payments
    ADD CONSTRAINT fk_billing_payments_invoice_org FOREIGN KEY (invoice_id, organization_id) REFERENCES billing_invoices(id, organization_id) ON DELETE CASCADE;

ALTER TABLE customer_subscriptions
    ADD CONSTRAINT fk_customer_subscriptions_organization_id FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;

ALTER TABLE customer_subscriptions
    ADD CONSTRAINT fk_customer_subscriptions_plan_id FOREIGN KEY (plan_id) REFERENCES product_plans(id) ON DELETE RESTRICT;

ALTER TABLE product_plan_entitlements
    ADD CONSTRAINT fk_product_plan_entitlements_feature_id FOREIGN KEY (feature_id) REFERENCES product_features(id) ON DELETE RESTRICT;

ALTER TABLE product_plan_entitlements
    ADD CONSTRAINT fk_product_plan_entitlements_plan_id FOREIGN KEY (plan_id) REFERENCES product_plans(id) ON DELETE CASCADE;

ALTER TABLE product_plan_prices
    ADD CONSTRAINT fk_product_plan_prices_plan_id FOREIGN KEY (plan_id) REFERENCES product_plans(id) ON DELETE RESTRICT;

ALTER TABLE subscription_events
    ADD CONSTRAINT fk_subscription_events_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE subscription_events
    ADD CONSTRAINT fk_subscription_events_subscription_org FOREIGN KEY (subscription_id, organization_id) REFERENCES customer_subscriptions(id, organization_id) ON DELETE CASCADE;

WITH perms(permission_name, module, action, description) AS (
	VALUES
		('platform.billing.invoice.read', 'billing', 'invoice_read', 'Read billing invoices'),
		('platform.billing.invoice.manage', 'billing', 'invoice_manage', 'Manage billing invoices'),
		('platform.billing.payment.manage', 'billing', 'payment_manage', 'Manage billing payments'),
		('platform.product.plan.read', 'product', 'plan_read', 'Read billing plan catalog'),
		('platform.product.plan.manage', 'product', 'plan_manage', 'Manage billing plans and prices'),
		('platform.product.plan_price.read', 'product', 'plan_price_read', 'Read billing plan prices'),
		('platform.product.plan_price.manage', 'product', 'plan_price_manage', 'Manage billing plan prices'),
		('platform.product.entitlement.read', 'product', 'entitlement_read', 'Read billing plan entitlements'),
		('platform.product.entitlement.manage', 'product', 'entitlement_manage', 'Manage billing plan entitlements'),
		('platform.subscription.read', 'subscription', 'subscription_read', 'Read organization subscriptions'),
		('platform.subscription.manage', 'subscription', 'subscription_manage', 'Manage organization subscriptions')
),
upserted AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM perms
	ON CONFLICT (slug) DO UPDATE SET updated_at = now()
	RETURNING id
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT r.id, u.id, 'all', now()
FROM roles r
CROSS JOIN upserted u
WHERE r.role_name = 'super_admin' OR r.slug = 'super_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;
