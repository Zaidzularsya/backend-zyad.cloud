package repository

import (
	"strings"
	"testing"

	"zyad.cloud/internal/modules/organization/model"
)

func TestCanonicalHost(t *testing.T) {
	if got := canonicalHost("  WWW.Example.COM. "); got != "www.example.com" {
		t.Fatalf("canonicalHost() = %q", got)
	}
}

func TestDomainWhereExcludesDeletedByDefault(t *testing.T) {
	where, args := domainWhere(DomainListFilter{
		Status: model.DomainStatusActive,
	})
	if !strings.Contains(where, "deleted_at IS NULL") ||
		!strings.Contains(where, "status = $1") {
		t.Fatalf("domainWhere() = %q", where)
	}
	if len(args) != 1 || args[0] != string(model.DomainStatusActive) {
		t.Fatalf("domainWhere() args = %v", args)
	}
}
