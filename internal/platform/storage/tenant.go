package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
)

var ErrInvalidTenantObject = errors.New("invalid tenant storage object")

type ObjectClass string

const (
	ObjectClassPrivate ObjectClass = "private"
	ObjectClassPublic  ObjectClass = "public"
	ObjectClassTemp    ObjectClass = "temp"
)

type ObjectKeyInput struct {
	Scope     coretenant.Scope
	Class     ObjectClass
	Namespace string
	Filename  string
	Extension string
}

type SignedOperation string

const (
	SignedOperationUpload   SignedOperation = "upload"
	SignedOperationDownload SignedOperation = "download"
)

type SignedObjectRequest struct {
	Scope     coretenant.Scope
	ObjectKey string
	Operation SignedOperation
}

type PublicAsset struct {
	OrganizationID string
	ObjectKey      string
	PublicID       string
	Published      bool
}

func OrganizationPrefix(scope coretenant.Scope) (string, error) {
	if !scope.IsValid() {
		return "", ErrInvalidTenantObject
	}
	return "organizations/" + scope.OrganizationID(), nil
}

func BuildObjectKey(input ObjectKeyInput, randomID string) (string, error) {
	prefix, err := OrganizationPrefix(input.Scope)
	if err != nil {
		return "", err
	}
	class := canonicalStorageSegment(string(input.Class))
	if !input.Class.IsValid() || class == "" {
		return "", ErrInvalidTenantObject
	}
	namespace := canonicalStorageSegment(input.Namespace)
	randomID = canonicalStorageSegment(randomID)
	if namespace == "" || randomID == "" {
		return "", ErrInvalidTenantObject
	}
	extension := normalizedExtension(input.Extension)
	if extension == "" {
		extension = normalizedExtension(path.Ext(input.Filename))
	}
	objectName := randomID
	if extension != "" {
		objectName += extension
	}
	return strings.Join([]string{prefix, class, namespace, objectName}, "/"), nil
}

func NewRandomObjectKey(input ObjectKeyInput) (string, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return BuildObjectKey(input, hex.EncodeToString(token))
}

func ValidateObjectOwnership(scope coretenant.Scope, objectKey string) error {
	prefix, err := OrganizationPrefix(scope)
	if err != nil {
		return err
	}
	objectKey = cleanObjectKey(objectKey)
	if objectKey == "" || !strings.HasPrefix(objectKey, prefix+"/") {
		return ErrInvalidTenantObject
	}
	return nil
}

func ValidateSignedObjectRequest(request SignedObjectRequest) error {
	if !request.Operation.IsValid() {
		return coreerrors.New(
			"STORAGE_SIGNED_OPERATION_INVALID",
			"signed storage operation is invalid",
			http.StatusBadRequest,
		)
	}
	if err := ValidateObjectOwnership(request.Scope, request.ObjectKey); err != nil {
		return coreerrors.New(
			"STORAGE_OBJECT_OWNERSHIP_INVALID",
			"storage object does not belong to organization",
			http.StatusForbidden,
		)
	}
	return nil
}

func PublicAssetPath(scope coretenant.Scope, asset PublicAsset) (string, error) {
	if !asset.Published {
		return "", coreerrors.New(
			"STORAGE_ASSET_NOT_PUBLISHED",
			"storage asset is not published",
			http.StatusForbidden,
		)
	}
	if err := scope.ValidateOrganization(asset.OrganizationID); err != nil {
		return "", coreerrors.New(
			"STORAGE_ASSET_OWNERSHIP_INVALID",
			"storage asset does not belong to organization",
			http.StatusForbidden,
		)
	}
	if err := ValidateObjectOwnership(scope, asset.ObjectKey); err != nil {
		return "", coreerrors.New(
			"STORAGE_OBJECT_OWNERSHIP_INVALID",
			"storage object does not belong to organization",
			http.StatusForbidden,
		)
	}
	publicID := canonicalStorageSegment(asset.PublicID)
	if publicID == "" {
		return "", coreerrors.New(
			"STORAGE_PUBLIC_ID_INVALID",
			"storage public id is invalid",
			http.StatusBadRequest,
		)
	}
	return "/public/assets/" + publicID, nil
}

// isPublicObjectKey memastikan key menunjuk object berkelas public milik sebuah
// organization: organizations/<id>/public/<namespace>/<name>.
func isPublicObjectKey(cleanedKey string) bool {
	segments := strings.Split(cleanedKey, "/")
	return len(segments) >= 4 &&
		segments[0] == "organizations" &&
		segments[2] == string(ObjectClassPublic)
}

func (c ObjectClass) IsValid() bool {
	switch c {
	case ObjectClassPrivate, ObjectClassPublic, ObjectClassTemp:
		return true
	default:
		return false
	}
}

func (o SignedOperation) IsValid() bool {
	return o == SignedOperationUpload || o == SignedOperationDownload
}

func canonicalStorageSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.Trim(value, ".")
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func cleanObjectKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return ""
	}
	return cleaned
}

func normalizedExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	value = canonicalStorageSegment(strings.TrimPrefix(value, "."))
	if value == "" {
		return ""
	}
	return "." + value
}
