package migration

import (
	"path/filepath"
	"testing"
)

func TestLoadIncludesMultiTenantFoundationMigrations(t *testing.T) {
	migrations, err := Load(filepath.Join("..", "..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expected := map[string]string{
		"000013": "create_organizations",
		"000014": "create_organization_memberships",
		"000015": "create_organization_domains",
		"000016": "create_organization_entitlements",
		"000017": "add_session_organization_context",
		"000018": "add_tenant_audit_metadata",
		"000019": "add_rls_foundation",
		"000020": "seed_platform_organization_permissions",
		"000021": "seed_organization_self_permissions",
		"000022": "seed_organization_domain_permission",
		"000025": "create_landing_page_core_tables",
		"000026": "create_landing_form_tables",
		"000027": "create_landing_version_tables",
		"000028": "create_landing_branding_tables",
		"000029": "create_landing_analytics_tables",
		"000030": "create_landing_reusable_tables",
		"000031": "create_landing_media_tables",
		"000032": "create_landing_revision_tables",
		"000033": "create_landing_integration_tables",
	}
	for _, migration := range migrations {
		if name, ok := expected[migration.Version]; ok && name == migration.Name {
			delete(expected, migration.Version)
		}
	}

	if len(expected) > 0 {
		t.Fatalf("migrations were not discovered: %#v", expected)
	}
}

