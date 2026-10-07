package repository

import "strings"

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func canonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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
