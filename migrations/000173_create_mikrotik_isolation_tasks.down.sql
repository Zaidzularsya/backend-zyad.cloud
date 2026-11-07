DROP TABLE IF EXISTS mikrotik_isolation_tasks;
ALTER TABLE mikrotik_ppp_secrets DROP CONSTRAINT IF EXISTS mikrotik_ppp_secrets_org_id_unique;
