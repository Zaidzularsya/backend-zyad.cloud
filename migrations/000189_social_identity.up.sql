-- Social identity (stage 1): referral code on users, customer links (user <-> CRM contact),
-- claim invites, per-tenant social settings, and attribution events.
--
-- RLS exception: customer_links, customer_claim_invites, social_tenant_settings and
-- attribution_events are intentionally GLOBAL tables WITHOUT apply_organization_rls (same pattern as
-- organization_domains). They are read cross-tenant by user id or token hash (e.g. "which companies
-- am I a customer of", claim by token), which RLS scoped to app.organization_id cannot express.
-- Tenant-side queries MUST filter organization_id explicitly in the repository.

ALTER TABLE users
	ADD COLUMN IF NOT EXISTS referral_code varchar(8),
	ADD COLUMN IF NOT EXISTS referred_by_user_id uuid;

ALTER TABLE users
	ADD CONSTRAINT users_referral_code_unique UNIQUE (referral_code),
	ADD CONSTRAINT fk_users_referred_by_user_id
		FOREIGN KEY (referred_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
	ADD CONSTRAINT users_referred_by_not_self_check CHECK (referred_by_user_id <> id);

CREATE TABLE IF NOT EXISTS customer_links (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	crm_contact_id uuid NOT NULL,
	status varchar(20) NOT NULL,
	verified_via varchar(20) NOT NULL,
	requested_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	activated_at timestamp without time zone,
	revoked_at timestamp without time zone,
	revoked_by uuid,
	CONSTRAINT customer_links_status_check CHECK (
		status IN ('pending_user', 'active', 'revoked', 'declined')
	),
	CONSTRAINT customer_links_verified_via_check CHECK (
		verified_via IN ('email', 'invite', 'staff')
	),
	CONSTRAINT fk_customer_links_user_id
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	CONSTRAINT fk_customer_links_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_customer_links_requested_by
		FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_customer_links_revoked_by
		FOREIGN KEY (revoked_by) REFERENCES users(id) ON DELETE SET NULL
);

-- One live link per CRM contact, and one live link per (user, organization).
CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_links_contact_live_unique
	ON customer_links (organization_id, crm_contact_id)
	WHERE status IN ('pending_user', 'active');

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_links_user_org_live_unique
	ON customer_links (user_id, organization_id)
	WHERE status IN ('pending_user', 'active');

CREATE TABLE IF NOT EXISTS customer_claim_invites (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	crm_contact_id uuid NOT NULL,
	email varchar(255) NOT NULL,
	token_hash text NOT NULL,
	expires_at timestamp without time zone NOT NULL,
	used_at timestamp without time zone,
	used_by_user_id uuid,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT customer_claim_invites_token_hash_unique UNIQUE (token_hash),
	CONSTRAINT fk_customer_claim_invites_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_customer_claim_invites_used_by_user_id
		FOREIGN KEY (used_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_customer_claim_invites_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_customer_claim_invites_contact
	ON customer_claim_invites (organization_id, crm_contact_id);

CREATE TABLE IF NOT EXISTS social_tenant_settings (
	organization_id uuid PRIMARY KEY,
	customer_claim_auto_offer boolean NOT NULL DEFAULT true,
	updated_by uuid,
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_social_tenant_settings_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_social_tenant_settings_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS attribution_events (
	id bigserial PRIMARY KEY,
	event varchar(20) NOT NULL,
	referral_code varchar(8) NOT NULL,
	referrer_user_id uuid NOT NULL,
	subject_user_id uuid,
	channel varchar(20),
	post_id uuid,
	ip_hash varchar(64),
	user_agent varchar(200),
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT attribution_events_event_check CHECK (
		event IN ('visit', 'signup', 'became_customer')
	),
	CONSTRAINT fk_attribution_events_referrer_user_id
		FOREIGN KEY (referrer_user_id) REFERENCES users(id) ON DELETE CASCADE,
	CONSTRAINT fk_attribution_events_subject_user_id
		FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_attribution_events_referrer_event
	ON attribution_events (referrer_user_id, event);

-- Cross-org email lookup on crm_contacts under FORCE RLS (same approach as migration 000167).
--
-- Claiming a customer link must find CRM contacts with a given email across ALL organizations.
-- Inside a transaction that sets app.crm_contact_email_lookup = '<normalized email>', non-deleted
-- contacts whose lower(btrim(email)) equals it become readable (SELECT only). Writes stay strictly
-- organization-scoped: the restrictive boundary keeps WITH CHECK strict and the only permissive
-- policy covering UPDATE/DELETE/INSERT is still organization_tenant_access.
-- Policy names are kept so remove_organization_rls('crm_contacts') keeps working.
DROP POLICY IF EXISTS organization_isolation_boundary ON crm_contacts;
CREATE POLICY organization_isolation_boundary ON crm_contacts
	AS RESTRICTIVE FOR ALL TO PUBLIC
	USING (
		app_organization_matches(organization_id)
		OR (
			deleted_at IS NULL
			AND email IS NOT NULL
			AND lower(btrim(email)) = current_setting('app.crm_contact_email_lookup', true)
		)
	)
	WITH CHECK (app_organization_matches(organization_id));

DROP POLICY IF EXISTS crm_contacts_email_lookup ON crm_contacts;
CREATE POLICY crm_contacts_email_lookup ON crm_contacts
	AS PERMISSIVE FOR SELECT TO PUBLIC
	USING (
		deleted_at IS NULL
		AND email IS NOT NULL
		AND lower(btrim(email)) = current_setting('app.crm_contact_email_lookup', true)
	);

CREATE INDEX IF NOT EXISTS idx_crm_contacts_email_lookup
	ON crm_contacts (lower(btrim(email)))
	WHERE deleted_at IS NULL AND email IS NOT NULL;
