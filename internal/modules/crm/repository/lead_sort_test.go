package repository

import "testing"

func TestLeadOrderBy(t *testing.T) {
	cases := map[string]string{
		"":              "created_at DESC, id DESC",
		"-created_at":   "created_at DESC, id DESC",
		"contact_name":  "lower(contact_name) ASC, id ASC",
		"-score":        "score DESC, id DESC",
		"unknown; DROP": "created_at DESC, id DESC",
	}
	for sort, want := range cases {
		if got := leadOrderBy(sort); got != want {
			t.Errorf("leadOrderBy(%q) = %q, want %q", sort, got, want)
		}
	}
}

func TestIsValidLeadSort(t *testing.T) {
	for _, ok := range []string{"", "status", "-status", "updated_at"} {
		if !IsValidLeadSort(ok) {
			t.Errorf("IsValidLeadSort(%q) = false", ok)
		}
	}
	for _, bad := range []string{"email", "--score", "score desc"} {
		if IsValidLeadSort(bad) {
			t.Errorf("IsValidLeadSort(%q) = true", bad)
		}
	}
}
