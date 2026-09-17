package repository

import (
	"fmt"
	"math/big"
	"strings"
)

func nullableUUID(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func amountOrZero(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	return value
}

// parseMoneyForCompare parses a numeric string as an exact rational for
// magnitude comparison (e.g. deciding a status transition). It never goes
// through float64, so it carries no rounding error regardless of amount size.
func parseMoneyForCompare(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return new(big.Rat), nil
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("finance: invalid amount %q", value)
	}
	return parsed, nil
}

func pagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
