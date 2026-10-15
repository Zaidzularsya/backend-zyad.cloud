package service

import "testing"

func TestNormalizeCompanyName(t *testing.T) {
	cases := map[string]string{
		"PT Maju Jaya":        "maju jaya",
		"pt. MAJU JAYA, Tbk":  "maju jaya",
		"CV  Sumber-Rejeki":   "sumber rejeki",
		"UD. Tani Makmur":     "tani makmur",
		"Maju Jaya (Persero)": "maju jaya persero",
		"PT":                  "",
		"  ":                  "",
		"100%_Diskon":         "100 diskon",
	}
	for in, want := range cases {
		if got := NormalizeCompanyName(in); got != want {
			t.Errorf("NormalizeCompanyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSimilarCompanyName(t *testing.T) {
	if !isSimilarCompanyName("maju jaya", "PT Maju Jaya Abadi") {
		t.Error("containment must match")
	}
	if isSimilarCompanyName("maju jaya", "PT Jaya Makmur") {
		t.Error("different names must not match")
	}
}
