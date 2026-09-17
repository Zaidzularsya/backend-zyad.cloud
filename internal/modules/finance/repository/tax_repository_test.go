package repository

import (
	"testing"

	"zyad.cloud/internal/modules/finance/model"
)

// TestTaxAccountIsDebited pins down the four combinations of tax type
// normal_balance x direction so the Dr/Cr side of the tax journal entry is
// never accidentally flipped — a flip here would silently misstate a
// liability as an asset (or vice versa) on the balance sheet.
func TestTaxAccountIsDebited(t *testing.T) {
	cases := []struct {
		name          string
		direction     model.TaxDirection
		normalBalance string
		want          bool
	}{
		{"increase a liability-normal tax account credits it", model.TaxDirectionIncrease, "credit", false},
		{"decrease a liability-normal tax account debits it", model.TaxDirectionDecrease, "credit", true},
		{"increase an asset-normal tax account debits it", model.TaxDirectionIncrease, "debit", true},
		{"decrease an asset-normal tax account credits it", model.TaxDirectionDecrease, "debit", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := taxAccountIsDebited(tc.direction, tc.normalBalance)
			if got != tc.want {
				t.Fatalf("taxAccountIsDebited(%s, %s) = %v, want %v", tc.direction, tc.normalBalance, got, tc.want)
			}
		})
	}
}
