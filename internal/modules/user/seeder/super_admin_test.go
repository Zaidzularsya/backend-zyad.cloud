package seeder

import "testing"

func TestSplitPermissionSlug(t *testing.T) {
	module, action := splitPermissionSlug("user.update_status")
	if module != "user" {
		t.Fatalf("expected module user, got %s", module)
	}
	if action != "update_status" {
		t.Fatalf("expected action update_status, got %s", action)
	}
}

func TestSplitPermissionSlugFallback(t *testing.T) {
	module, action := splitPermissionSlug("invalid")
	if module != "permission" {
		t.Fatalf("expected fallback module permission, got %s", module)
	}
	if action != "manage" {
		t.Fatalf("expected fallback action manage, got %s", action)
	}
}
