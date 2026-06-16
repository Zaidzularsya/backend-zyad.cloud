CREATE OR REPLACE FUNCTION app_current_organization_id()
RETURNS uuid
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
	SELECT NULLIF(
		btrim(current_setting('app.organization_id', true)),
		''
	)::uuid;
$$;

CREATE OR REPLACE FUNCTION app_organization_matches(row_organization_id uuid)
RETURNS boolean
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
	SELECT
		row_organization_id IS NOT NULL
		AND row_organization_id = app_current_organization_id();
$$;

CREATE OR REPLACE FUNCTION apply_organization_rls(target_table regclass)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	organization_column_is_not_null boolean;
BEGIN
	SELECT attribute.attnotnull
	INTO organization_column_is_not_null
	FROM pg_attribute attribute
	WHERE attribute.attrelid = target_table
		AND attribute.attname = 'organization_id'
		AND attribute.attnum > 0
		AND NOT attribute.attisdropped;

	IF organization_column_is_not_null IS NULL THEN
		RAISE EXCEPTION 'table % must have an organization_id column', target_table;
	END IF;
	IF NOT organization_column_is_not_null THEN
		RAISE EXCEPTION 'table %.organization_id must be NOT NULL', target_table;
	END IF;

	EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', target_table);
	EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', target_table);
	EXECUTE format(
		'DROP POLICY IF EXISTS organization_isolation_boundary ON %s',
		target_table
	);
	EXECUTE format(
		'DROP POLICY IF EXISTS organization_tenant_access ON %s',
		target_table
	);
	EXECUTE format(
		'CREATE POLICY organization_isolation_boundary ON %s '
		'AS RESTRICTIVE FOR ALL TO PUBLIC '
		'USING (app_organization_matches(organization_id)) '
		'WITH CHECK (app_organization_matches(organization_id))',
		target_table
	);
	EXECUTE format(
		'CREATE POLICY organization_tenant_access ON %s '
		'AS PERMISSIVE FOR ALL TO PUBLIC '
		'USING (app_organization_matches(organization_id)) '
		'WITH CHECK (app_organization_matches(organization_id))',
		target_table
	);
END;
$$;

CREATE OR REPLACE FUNCTION remove_organization_rls(target_table regclass)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
	EXECUTE format(
		'DROP POLICY IF EXISTS organization_isolation_boundary ON %s',
		target_table
	);
	EXECUTE format(
		'DROP POLICY IF EXISTS organization_tenant_access ON %s',
		target_table
	);
	EXECUTE format('ALTER TABLE %s NO FORCE ROW LEVEL SECURITY', target_table);
	EXECUTE format('ALTER TABLE %s DISABLE ROW LEVEL SECURITY', target_table);
END;
$$;

REVOKE EXECUTE ON FUNCTION apply_organization_rls(regclass) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION remove_organization_rls(regclass) FROM PUBLIC;
