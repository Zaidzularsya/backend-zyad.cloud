package repository

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrOrganizationScopeRequired is returned by List() when no OrganizationID
// filter is set and AllOrganizations wasn't explicitly opted into — this
// keeps "forgot to scope the query" from silently becoming a cross-tenant
// data leak.
var ErrOrganizationScopeRequired = errors.New(
	"organization_id is required unless listing across all organizations",
)

func encodeMap(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeMap(data []byte, target *map[string]any) error {
	if len(data) == 0 {
		*target = map[string]any{}
		return nil
	}
	return json.Unmarshal(data, target)
}

func stringPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func canonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func upperOrDefault(value string, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func pagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
