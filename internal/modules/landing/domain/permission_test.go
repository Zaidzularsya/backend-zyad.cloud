package domain

import (
	"strings"
	"testing"
)

func TestPermissionsAreUniqueAndUseLandingPrefix(t *testing.T) {
	seen := make(map[string]struct{}, len(Permissions))

	for _, permission := range Permissions {
		if !strings.HasPrefix(permission, "landing.") {
			t.Fatalf("permission %q does not use landing prefix", permission)
		}
		if _, exists := seen[permission]; exists {
			t.Fatalf("permission %q is duplicated", permission)
		}
		seen[permission] = struct{}{}
	}
}
