package logger

import (
	"log/slog"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

func TestTenantAttrsIncludesStructuredTenantAndActorFields(t *testing.T) {
	tenantContext := loggerTestTenantContext(t, coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})

	fields := attrsMap(TenantAttrs(tenantContext, TenantFieldContext{
		RequestID: "request-1",
		UserID:    "33333333-3333-3333-3333-333333333333",
		SessionID: "44444444-4444-4444-4444-444444444444",
	}))
	if fields["organization_id"] != "11111111-1111-1111-1111-111111111111" ||
		fields["membership_id"] != "22222222-2222-2222-2222-222222222222" ||
		fields["request_id"] != "request-1" ||
		fields["operator_user_id"] != "33333333-3333-3333-3333-333333333333" ||
		fields["effective_user_id"] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("TenantAttrs() = %#v", fields)
	}
}

func TestTenantAttrsIncludesImpersonationActors(t *testing.T) {
	tenantContext := loggerTestTenantContext(t, coretenant.VerifiedContextInput{
		OrganizationID:         "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:       "acme",
		OrganizationType:       coretenant.OrganizationTypeCustomer,
		OrganizationStatus:     coretenant.OrganizationStatusActive,
		ResolutionSource:       coretenant.ResolutionSourceInternal,
		DataPlacement:          coretenant.DataPlacementShared,
		IsPlatformOperator:     true,
		ImpersonationSessionID: "55555555-5555-5555-5555-555555555555",
	})

	fields := attrsMap(TenantAttrs(tenantContext, TenantFieldContext{
		OperatorUserID:  "66666666-6666-6666-6666-666666666666",
		EffectiveUserID: "77777777-7777-7777-7777-777777777777",
	}))
	if fields["operator_user_id"] != "66666666-6666-6666-6666-666666666666" ||
		fields["effective_user_id"] != "77777777-7777-7777-7777-777777777777" ||
		fields["impersonation_session_id"] != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("TenantAttrs() = %#v", fields)
	}
}

func TestTenantAttrsRejectsInvalidContext(t *testing.T) {
	if attrs := TenantAttrs(coretenant.Context{}, TenantFieldContext{}); attrs != nil {
		t.Fatalf("TenantAttrs() = %#v, want nil", attrs)
	}
}

func loggerTestTenantContext(
	t *testing.T,
	input coretenant.VerifiedContextInput,
) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(input)
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}

func attrsMap(attrs []slog.Attr) map[string]string {
	fields := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		fields[attr.Key] = attr.Value.String()
	}
	return fields
}
