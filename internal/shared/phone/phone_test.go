package phone

import "testing"

// normalizeCases is shared with phone_integration_test.go, which checks the
// SQL function normalize_phone_id returns the same results.
var normalizeCases = []struct {
	Raw  string
	Want string
	OK   bool
}{
	{"0812-3456-7890", "6281234567890", true},
	{"+62 812 3456 7890", "6281234567890", true},
	{"62-812-3456-7890", "6281234567890", true},
	{"81234567890", "6281234567890", true},
	{"(021) 555-1234", "62215551234", true},
	{"+1 (555) 123-4567", "15551234567", true},
	{"0062 812 3456 7890", "6281234567890", true},
	{"", "", false},
	{"abc", "", false},
	{"0812", "", false},
	{"+62 812 3456 7890 1234", "", false},
}

func TestNormalizeID(t *testing.T) {
	for _, tc := range normalizeCases {
		got, ok := NormalizeID(tc.Raw)
		if got != tc.Want || ok != tc.OK {
			t.Errorf("NormalizeID(%q) = (%q, %v), want (%q, %v)", tc.Raw, got, ok, tc.Want, tc.OK)
		}
	}
}
