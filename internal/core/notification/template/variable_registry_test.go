package template

import "testing"

func TestVariableRegistryGetReturnsCopy(t *testing.T) {
	registry := NewVariableRegistry()

	variables := registry.Get("auth.password_reset")
	if len(variables) != 4 {
		t.Fatalf("Get() length = %d, want 4", len(variables))
	}
	if variables[0].Key != "app_name" {
		t.Fatalf("Get() first key = %q, want app_name", variables[0].Key)
	}

	variables[0].Key = "changed"
	again := registry.Get("auth.password_reset")
	if again[0].Key != "app_name" {
		t.Fatalf("Get() returned mutable registry data, got key %q", again[0].Key)
	}
}

func TestVariableRegistryHas(t *testing.T) {
	registry := NewVariableRegistry()

	if !registry.Has("lead.created", "crm_url") {
		t.Fatal("Has() crm_url = false, want true")
	}
	if registry.Has("lead.created", "unknown") {
		t.Fatal("Has() unknown = true, want false")
	}
	if registry.Has("unknown.template", "crm_url") {
		t.Fatal("Has() unknown template = true, want false")
	}
}

func TestVariableRegistryCodes(t *testing.T) {
	registry := NewVariableRegistry()

	codes := registry.Codes()
	want := []string{
		"auth.password_changed",
		"auth.password_reset",
		"lead.created",
		"payment.paid",
		"permission.updated",
		"security.new_login",
		"user.invitation",
	}
	if len(codes) != len(want) {
		t.Fatalf("Codes() length = %d, want %d", len(codes), len(want))
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("Codes()[%d] = %q, want %q", i, codes[i], want[i])
		}
	}
}
