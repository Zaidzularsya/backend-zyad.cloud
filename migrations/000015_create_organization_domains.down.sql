DROP TRIGGER IF EXISTS trg_reject_claimed_reserved_subdomain ON reserved_subdomains;
DROP FUNCTION IF EXISTS reject_claimed_reserved_subdomain();
DROP TRIGGER IF EXISTS trg_reject_reserved_organization_subdomain ON organization_domains;
DROP FUNCTION IF EXISTS reject_reserved_organization_subdomain();
DROP TABLE IF EXISTS organization_domains;
DROP FUNCTION IF EXISTS canonical_hostname_is_valid(text);
DROP TABLE IF EXISTS reserved_subdomains;
