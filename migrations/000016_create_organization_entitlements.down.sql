DROP TRIGGER IF EXISTS trg_increment_usage_counter_version ON organization_usage_counters;
DROP FUNCTION IF EXISTS increment_usage_counter_version();

DROP TRIGGER IF EXISTS trg_increment_entitlement_version ON organization_entitlements;
DROP FUNCTION IF EXISTS increment_entitlement_version();

DROP TABLE IF EXISTS organization_usage_counters;
DROP TABLE IF EXISTS organization_entitlements;
