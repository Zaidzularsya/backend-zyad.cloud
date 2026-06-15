package repository

import (
	"strings"
	"testing"

	"zyad.cloud/internal/modules/organization/model"
)

func TestMembershipWhereExcludesRemovedByDefault(t *testing.T) {
	where, args := membershipWhere(MembershipListFilter{})
	if !strings.Contains(where, "status <> 'removed'") ||
		!strings.Contains(where, "removed_at IS NULL") {
		t.Fatalf("membershipWhere() = %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("membershipWhere() args = %v, want empty", args)
	}
}

func TestMembershipWhereStatusFilter(t *testing.T) {
	where, args := membershipWhere(MembershipListFilter{
		Status: model.MembershipStatusActive,
	})
	if !strings.Contains(where, "status = $1") {
		t.Fatalf("membershipWhere() = %q", where)
	}
	if len(args) != 1 || args[0] != string(model.MembershipStatusActive) {
		t.Fatalf("membershipWhere() args = %v", args)
	}
}
