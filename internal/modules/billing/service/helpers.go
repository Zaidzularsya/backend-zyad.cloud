package service

import (
	"strings"
	"time"
)

func normalizePage(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		parsed, err = time.Parse(time.DateOnly, trimmed)
	}
	if err != nil {
		return nil, validationError("time value must use RFC3339 or date format")
	}
	utc := parsed.UTC()
	return &utc, nil
}

func defaultPeriodEnd(start time.Time, interval string) *time.Time {
	var end time.Time
	switch interval {
	case "yearly":
		end = start.AddDate(1, 0, 0)
	case "custom":
		return nil
	default:
		end = start.AddDate(0, 1, 0)
	}
	return &end
}
