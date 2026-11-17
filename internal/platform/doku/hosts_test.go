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

// TestCheckoutHosts_Production is the go-live gate for isolir in production.
// DOKU's docs do not state the production checkout page host: their sandbox
// example (sandbox.doku.com/checkout-link-v2) differs from what a real sandbox
// checkout redirected to (staging.doku.com), and the only production hint is
// jokul.doku.com (the production Checkout JS host and an unlabelled sample
// link). Asset hosts for production are not documented at all. Replace this
// skip with real assertions once a production checkout has been observed
// (redirect target and every asset host the page loads).
func TestCheckoutHosts_Production(t *testing.T) {
	t.Skip("host produksi belum terkonfirmasi: go-live isolir produksi diblokir sampai checkout produksi nyata diamati")
}
