package service

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance"
)

const dateLayout = "2006-01-02"

func normalizePage(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 200 {
		perPage = 20
	}
	return page, perPage
}

func moneyRat(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return new(big.Rat), nil
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, finance.ValidationError("invalid amount: " + value)
	}
	return parsed, nil
}

func formatMoney(value *big.Rat) string {
	if value == nil {
		return "0.00"
	}
	return value.FloatString(2)
}

func parseRequiredDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, finance.ValidationError("date is required")
	}
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, finance.ValidationError(fmt.Sprintf("invalid date %q, expected YYYY-MM-DD", value))
	}
	return parsed, nil
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(dateLayout)
}

func parseOptionalDate(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := parseRequiredDate(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func formatOptionalDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatDate(*value)
	return &formatted
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}

func trimmedOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func totalPagesOf(perPage int, total int64) int {
	if total <= 0 {
		return 0
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}
