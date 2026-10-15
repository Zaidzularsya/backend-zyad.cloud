package service

import (
	"math/big"
	"strings"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

type QuotationTotals = pricing.Totals

func parseNonNegative(value, fallback string) (*big.Rat, bool) {
	r, err := parseDecimalOrDefault(value, fallback)
	if err != nil || r.Sign() < 0 {
		return nil, false
	}
	return r, true
}

// priceQuotationLines membungkus pricing.PriceLines dan menambah cabang legacy:
// legacyTaxTotal dipakai hanya bila tidak ada baris berpajak (kontrak lama
// POST /quotations dengan tax_total header). Input tidak valid → ErrInvalidQuotationAmount.
func priceQuotationLines(lines []QuotationLineInput, legacyTaxTotal string) (QuotationTotals, []repository.QuotationItemInput, error) {
	in := make([]pricing.LineInput, len(lines))
	for i, l := range lines {
		in[i] = pricing.LineInput{
			Description: l.Description, Quantity: l.Quantity, UnitPrice: l.UnitPrice,
			DiscountPercent: l.DiscountPercent, TaxPercent: l.TaxPercent, Attrs: l.Pricing,
		}
	}
	totals, priced, err := pricing.PriceLines(in)
	if err != nil {
		return QuotationTotals{}, nil, ErrInvalidQuotationAmount
	}
	items := make([]repository.QuotationItemInput, len(priced))
	anyLineTax := false
	for i, p := range priced {
		if r, _ := new(big.Rat).SetString(p.TaxPercent); r.Sign() > 0 {
			anyLineTax = true
		}
		items[i] = repository.QuotationItemInput{
			ProductID: lines[i].ProductID, Description: p.Description, SKU: strings.TrimSpace(lines[i].SKU), Unit: strings.TrimSpace(lines[i].Unit),
			Quantity: p.Quantity, UnitPrice: p.UnitPrice, DiscountPercent: p.DiscountPercent,
			TaxPercent: p.TaxPercent, TaxAmount: p.TaxAmount, LineTotal: p.LineTotal, Position: i, Pricing: p.Attrs, Features: lines[i].Features,
		}
	}
	if !anyLineTax && strings.TrimSpace(legacyTaxTotal) != "" {
		legacy, ok := parseNonNegative(legacyTaxTotal, "0")
		if !ok {
			return QuotationTotals{}, nil, ErrInvalidQuotationAmount
		}
		totals, _ = totals.WithTaxTotal(legacy.FloatString(2))
	}
	return totals, items, nil
}
