package model

import (
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

func TestOrganizationTypeHelpers(t *testing.T) {
	organization := Organization{
		Type:   coretenant.OrganizationTypePlatform,
		Status: coretenant.OrganizationStatusActive,
	}

	if !organization.IsPlatform() {
		t.Fatal("expected platform organization")
	}
	if !organization.IsActive() {
		t.Fatal("expected active organization")
	}
}
