package doku

import "strings"

// CheckoutHosts lists the hostnames a customer's browser must reach to open the
// DOKU hosted checkout page and its assets. A customer behind the isolir
// walled garden can only pay if all of them are allowlisted.
//
// The sandbox set is proven by a real checkout (the redirect goes to
// staging.doku.com, not sandbox.doku.com, and the page loads assets from an
// Alibaba OSS bucket). The production set is not confirmed yet, so it returns
// nil rather than a guess; see the go-live gate in the Gelombang 1 plan.
func CheckoutHosts(baseURL string) []string {
	if !strings.Contains(strings.ToLower(baseURL), "sandbox") {
		return nil
	}
	return []string{
		"api-sandbox.doku.com",
		"cdn-doku.oss-ap-southeast-5.aliyuncs.com",
		"sandbox.doku.com",
		"staging.doku.com",
	}
}
