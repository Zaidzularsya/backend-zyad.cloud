-- The router service records 'host_not_allowed' when a stored router host is no
-- longer allowed by the host policy (e.g. the allow list was narrowed). 000168
-- did not list it, so recording that outcome failed with SQLSTATE 23514.
ALTER TABLE mikrotik_routers DROP CONSTRAINT IF EXISTS mikrotik_routers_last_check_status_check;
ALTER TABLE mikrotik_routers ADD CONSTRAINT mikrotik_routers_last_check_status_check CHECK (
	last_check_status IS NULL OR last_check_status IN ('ok', 'unreachable', 'auth_failed', 'tls_untrusted', 'host_not_allowed')
);
