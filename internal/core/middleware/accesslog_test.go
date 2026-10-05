package middleware

import "testing"

func TestMaskPublicToken(t *testing.T) {
	cases := map[string]string{
		"/api/v1/public/quotations/abcDEF123_-xyz":         "/api/v1/public/quotations/***",
		"/api/v1/public/quotations/abcDEF123_-xyz/pdf":     "/api/v1/public/quotations/***/pdf",
		"/api/v1/public/quotations/abcDEF123_-xyz/approve": "/api/v1/public/quotations/***/approve",
		"/api/v1/app/crm/quotations/11111111/pdf":          "/api/v1/app/crm/quotations/11111111/pdf",
		"/api/v1/public/landing/pages":                     "/api/v1/public/landing/pages",
	}
	for in, want := range cases {
		if got := MaskPublicToken(in); got != want {
			t.Errorf("MaskPublicToken(%q) = %q, want %q", in, got, want)
		}
	}
}
