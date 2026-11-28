DROP INDEX IF EXISTS idx_crm_contacts_email_lookup;

DROP POLICY IF EXISTS crm_contacts_email_lookup ON crm_contacts;

-- Restore the boundary policy exactly as created by apply_organization_rls (migration 000019).
DROP POLICY IF EXISTS organization_isolation_boundary ON crm_contacts;
CREATE POLICY organization_isolation_boundary ON crm_contacts
	AS RESTRICTIVE FOR ALL TO PUBLIC
	USING (app_organization_matches(organization_id))
	WITH CHECK (app_organization_matches(organization_id));

DROP TABLE IF EXISTS attribution_events;
DROP TABLE IF EXISTS social_tenant_settings;
DROP TABLE IF EXISTS customer_claim_invites;
DROP TABLE IF EXISTS customer_links;

ALTER TABLE users
	DROP CONSTRAINT IF EXISTS users_referred_by_not_self_check,
	DROP CONSTRAINT IF EXISTS fk_users_referred_by_user_id,
	DROP CONSTRAINT IF EXISTS users_referral_code_unique;

ALTER TABLE users
	DROP COLUMN IF EXISTS referred_by_user_id,
	DROP COLUMN IF EXISTS referral_code;
