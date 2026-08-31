CREATE OR REPLACE FUNCTION reject_reserved_organization_subdomain()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	subdomain_label text;
BEGIN
	IF NEW.type <> 'subdomain' OR NEW.deleted_at IS NOT NULL THEN
		RETURN NEW;
	END IF;

	subdomain_label := split_part(NEW.canonical_host, '.', 1);
	IF EXISTS (
		SELECT 1
		FROM reserved_subdomains
		WHERE label = subdomain_label
	) THEN
		RAISE EXCEPTION 'subdomain label "%" is reserved', subdomain_label;
	END IF;

	RETURN NEW;
END;
$$;
