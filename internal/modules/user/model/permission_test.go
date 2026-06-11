package model

import "testing"

func TestPermissionEffectIsValid(t *testing.T) {
	if !PermissionEffectAllow.IsValid() {
		t.Fatal("expected allow effect to be valid")
	}
	if !PermissionEffectDeny.IsValid() {
		t.Fatal("expected deny effect to be valid")
	}
	if PermissionEffect("unknown").IsValid() {
		t.Fatal("expected unknown effect to be invalid")
	}
}
