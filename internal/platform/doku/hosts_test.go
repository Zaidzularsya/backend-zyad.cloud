package doku

import (
	"reflect"
	"testing"
)

func TestCheckoutHosts_Sandbox(t *testing.T) {
	want := []string{"api-sandbox.doku.com", "cdn-doku.oss-ap-southeast-5.aliyuncs.com", "sandbox.doku.com", "staging.doku.com"}
	for _, base := range []string{SandboxBaseURL, "HTTPS://API-SANDBOX.DOKU.COM/"} {
		if got := CheckoutHosts(base); !reflect.DeepEqual(got, want) {
			t.Fatalf("CheckoutHosts(%q) = %v, want %v", base, got, want)
		}
	}
}

func TestCheckoutHosts_ProductionUnconfirmed(t *testing.T) {
	if got := CheckoutHosts(ProductionBaseURL); len(got) != 0 {
		t.Fatalf("production hosts must stay empty until confirmed, got %v", got)
	}
}
