// Package phone normalizes free-text phone numbers for matching.
package phone

import "strings"

// NormalizeID returns the digits-only form used to match phone numbers across
// CRM and WhatsApp (Indonesian numbers become 62xxxxxxxxxx).
//
// It must stay identical to the SQL function normalize_phone_id (migration
// 000125), which fills crm_leads/crm_contacts.phone_normalized; an
// integration test compares both on the same cases:
//   - keep digits only;
//   - drop a "00" international prefix;
//   - leading "0" becomes "62", leading "8" (mobile without prefix) becomes "628";
//   - the result must be 8-15 digits, otherwise ("", false).
func NormalizeID(raw string) (string, bool) {
	var builder strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	digits := builder.String()

	switch {
	case strings.HasPrefix(digits, "00"):
		digits = digits[2:]
	case strings.HasPrefix(digits, "0"):
		digits = "62" + digits[1:]
	case strings.HasPrefix(digits, "8"):
		digits = "62" + digits
	}

	if len(digits) < 8 || len(digits) > 15 {
		return "", false
	}
	return digits, true
}
