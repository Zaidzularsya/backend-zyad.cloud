package repository

import "testing"

func TestCanonicalSlug(t *testing.T) {
	if got := canonicalSlug("  Acme-CLOUD  "); got != "acme-cloud" {
		t.Fatalf("canonicalSlug() = %q, want %q", got, "acme-cloud")
	}
}

func TestOrganizationWhereExcludesArchivedByDefault(t *testing.T) {
	where, args := organizationWhere(OrganizationListFilter{})
	if where != " WHERE 1 = 1 AND deleted_at IS NULL AND status <> 'archived'" {
		t.Fatalf("organizationWhere() = %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("organizationWhere() args = %v, want empty", args)
	}
}
