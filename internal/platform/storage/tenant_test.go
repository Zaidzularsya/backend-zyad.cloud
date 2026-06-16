package storage

import (
	"errors"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

const (
	storageTestOrganizationID  = "11111111-1111-1111-1111-111111111111"
	storageOtherOrganizationID = "22222222-2222-2222-2222-222222222222"
)

func TestBuildObjectKeyUsesOrganizationPrefixAndRandomName(t *testing.T) {
	key, err := BuildObjectKey(ObjectKeyInput{
		Scope:     storageTestScope(t, storageTestOrganizationID),
		Class:     ObjectClassPrivate,
		Namespace: "Landing Media",
		Filename:  "Hero Image.PNG",
	}, " Random Token ")
	if err != nil {
		t.Fatalf("BuildObjectKey() error = %v", err)
	}
	want := "organizations/" + storageTestOrganizationID +
		"/private/landing_media/random_token.png"
	if key != want {
		t.Fatalf("BuildObjectKey() = %q, want %q", key, want)
	}
	if strings.Contains(key, "hero") {
		t.Fatalf("object key exposed original filename: %q", key)
	}
}

func TestBuildObjectKeyRequiresTenantScope(t *testing.T) {
	_, err := BuildObjectKey(ObjectKeyInput{
		Class:     ObjectClassPrivate,
		Namespace: "landing",
		Filename:  "file.pdf",
	}, "token")
	if !errors.Is(err, ErrInvalidTenantObject) {
		t.Fatalf("BuildObjectKey() error = %v", err)
	}
}

func TestValidateSignedObjectRequestRejectsCrossTenantKey(t *testing.T) {
	err := ValidateSignedObjectRequest(SignedObjectRequest{
		Scope: storageTestScope(t, storageTestOrganizationID),
		ObjectKey: "organizations/" + storageOtherOrganizationID +
			"/private/landing/file.pdf",
		Operation: SignedOperationDownload,
	})
	if err == nil {
		t.Fatal("ValidateSignedObjectRequest() expected error")
	}
}

func TestValidateSignedObjectRequestAcceptsOwnedKey(t *testing.T) {
	err := ValidateSignedObjectRequest(SignedObjectRequest{
		Scope: storageTestScope(t, storageTestOrganizationID),
		ObjectKey: "organizations/" + storageTestOrganizationID +
			"/private/landing/file.pdf",
		Operation: SignedOperationDownload,
	})
	if err != nil {
		t.Fatalf("ValidateSignedObjectRequest() error = %v", err)
	}
}

func TestPublicAssetPathRequiresPublishedOwnedAsset(t *testing.T) {
	scope := storageTestScope(t, storageTestOrganizationID)
	path, err := PublicAssetPath(scope, PublicAsset{
		OrganizationID: storageTestOrganizationID,
		ObjectKey: "organizations/" + storageTestOrganizationID +
			"/public/landing/file.pdf",
		PublicID:  "Asset ABC",
		Published: true,
	})
	if err != nil {
		t.Fatalf("PublicAssetPath() error = %v", err)
	}
	if path != "/public/assets/asset_abc" {
		t.Fatalf("PublicAssetPath() = %q", path)
	}
	if strings.Contains(path, storageTestOrganizationID) ||
		strings.Contains(path, "organizations/") {
		t.Fatalf("public path exposed storage key: %q", path)
	}
}

func TestPublicAssetPathRejectsCrossTenantAssetMetadata(t *testing.T) {
	_, err := PublicAssetPath(storageTestScope(t, storageTestOrganizationID), PublicAsset{
		OrganizationID: storageOtherOrganizationID,
		ObjectKey: "organizations/" + storageTestOrganizationID +
			"/public/landing/file.pdf",
		PublicID:  "asset",
		Published: true,
	})
	if err == nil {
		t.Fatal("PublicAssetPath() expected error")
	}
}

func TestPublicAssetPathRejectsUnpublishedAsset(t *testing.T) {
	_, err := PublicAssetPath(storageTestScope(t, storageTestOrganizationID), PublicAsset{
		OrganizationID: storageTestOrganizationID,
		ObjectKey: "organizations/" + storageTestOrganizationID +
			"/public/landing/file.pdf",
		PublicID: "asset",
	})
	if err == nil {
		t.Fatal("PublicAssetPath() expected error")
	}
}

func storageTestScope(t *testing.T, organizationID string) coretenant.Scope {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     organizationID,
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
		MembershipID:       "33333333-3333-3333-3333-333333333333",
		MembershipStatus:   "active",
		MembershipVersion:  1,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}
