package dto

import (
	"reflect"
	"testing"
)

func TestTenantMutationRequestsDoNotExposeOrganizationID(t *testing.T) {
	requests := []any{
		UpdateCurrentOrganizationRequest{},
		InviteMemberRequest{},
		UpdateMembershipStatusRequest{},
		CreateDomainRequest{},
		UpdateDomainRequest{},
	}

	for _, request := range requests {
		requestType := reflect.TypeOf(request)
		if _, ok := requestType.FieldByName("OrganizationID"); ok {
			t.Fatalf("%s must derive organization ID from verified context", requestType.Name())
		}
	}
}

func TestSensitivePersistenceFieldsAreNotExposed(t *testing.T) {
	responseTypes := []any{
		OrganizationResponse{},
		DomainResponse{},
		EntitlementResponse{},
	}
	for _, response := range responseTypes {
		responseType := reflect.TypeOf(response)
		for _, fieldName := range []string{
			"ConnectionString",
			"ConnectionReference",
			"VerificationChallengeHash",
			"InvitationTokenHash",
		} {
			if _, ok := responseType.FieldByName(fieldName); ok {
				t.Fatalf("%s must not expose %s", responseType.Name(), fieldName)
			}
		}
	}
}
