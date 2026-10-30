-- Restore the 000168 set. Rows recorded as 'host_not_allowed' have no place in
-- it; 'unreachable' is the closest older status (the router could not be used).
-- mikrotik_routers is under FORCE RLS, so the update runs once per organization.
DO $$
DECLARE
	org uuid;
BEGIN
	FOR org IN SELECT id FROM organizations LOOP
		PERFORM set_config('app.organization_id', org::text, true);
		UPDATE mikrotik_routers SET last_check_status = 'unreachable'
		WHERE organization_id = org AND last_check_status = 'host_not_allowed';
	END LOOP;
END
$$;

ALTER TABLE mikrotik_routers DROP CONSTRAINT IF EXISTS mikrotik_routers_last_check_status_check;
ALTER TABLE mikrotik_routers ADD CONSTRAINT mikrotik_routers_last_check_status_check CHECK (
	last_check_status IS NULL OR last_check_status IN ('ok', 'unreachable', 'auth_failed', 'tls_untrusted')
);
