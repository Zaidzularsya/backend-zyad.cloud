package service

import (
	"math/big"
	"strings"
	"unicode/utf8"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/pricing"
)

type QuotationTotals struct {
	Subtotal      string
	DiscountTotal string
	TaxTotal      string
	GrandTotal    string
	// Rincian: total per baris = line_total + pajak. FirstInvoiceTotal = semua
	// baris prabayar (sekali bayar maupun berulang periode pertama).
	OneTimeTotal      string
	FirstInvoiceTotal string
	RecurringTotals   map[pricing.Frequency]string
}

var (
	ratZero    = new(big.Rat)
	ratHundred = big.NewRat(100, 1)
	ratHalf    = big.NewRat(1, 2)
)

// roundHalfUp2 membulatkan ke 2 desimal, setengah ke atas (nilai >= 0).
func roundHalfUp2(v *big.Rat) *big.Rat {
	scaled := new(big.Rat).Mul(v, ratHundred)
	scaled.Add(scaled, ratHalf)
	floor := new(big.Int).Quo(scaled.Num(), scaled.Denom()) // Num>=0 → Quo = floor
	return new(big.Rat).SetFrac(floor, big.NewInt(100))
}

func parseNonNegative(value, fallback string, max *big.Rat) (*big.Rat, bool) {
	r, err := parseDecimalOrDefault(value, fallback)
	if err != nil || r.Sign() < 0 || (max != nil && r.Cmp(max) > 0) {
		return nil, false
	}
	return r, true
}

// priceQuotationLines menghitung setiap baris (dibulatkan per baris) dan
// total header. Atribut harga tiap baris dinormalisasi (default one_time +
// prepaid); atribut tidak valid → ErrInvalidQuotationAmount. legacyTaxTotal dipakai hanya bila tidak ada baris berpajak
// (kontrak lama POST /quotations dengan tax_total header).
func priceQuotationLines(lines []QuotationLineInput, legacyTaxTotal string) (QuotationTotals, []repository.QuotationItemInput, error) {
	if len(lines) == 0 {
		return QuotationTotals{}, nil, ErrInvalidQuotationAmount
	}
	subtotal, discountTotal, taxTotal := new(big.Rat), new(big.Rat), new(big.Rat)
	items := make([]repository.QuotationItemInput, 0, len(lines))
	anyLineTax := false
	oneTime, firstInvoice := new(big.Rat), new(big.Rat)
	recurring := map[pricing.Frequency]*big.Rat{}
	for i, l := range lines {
		description := strings.TrimSpace(l.Description)
		qty, ok1 := parseNonNegative(l.Quantity, "1", nil)
		price, ok2 := parseNonNegative(l.UnitPrice, "0", nil)
		discPct, ok3 := parseNonNegative(l.DiscountPercent, "0", ratHundred)
		taxPct, ok4 := parseNonNegative(l.TaxPercent, "0", ratHundred)
		attrs, attrErr := pricing.Normalize(l.Pricing)
		if attrErr != nil || description == "" || utf8.RuneCountInString(description) > 500 || !ok1 || !ok2 || !ok3 || !ok4 {
			return QuotationTotals{}, nil, ErrInvalidQuotationAmount
		}
		gross := roundHalfUp2(new(big.Rat).Mul(qty, price))
		disc := roundHalfUp2(new(big.Rat).Quo(new(big.Rat).Mul(gross, discPct), ratHundred))
		net := new(big.Rat).Sub(gross, disc)
		tax := roundHalfUp2(new(big.Rat).Quo(new(big.Rat).Mul(net, taxPct), ratHundred))
		if taxPct.Cmp(ratZero) > 0 {
			anyLineTax = true
		}
		subtotal.Add(subtotal, gross)
		discountTotal.Add(discountTotal, disc)
		taxTotal.Add(taxTotal, tax)

		lineGross := new(big.Rat).Add(net, tax)
		if attrs.ChargeType == pricing.Recurring {
			if recurring[attrs.Frequency] == nil {
				recurring[attrs.Frequency] = new(big.Rat)
			}
			recurring[attrs.Frequency].Add(recurring[attrs.Frequency], lineGross)
		} else {
			oneTime.Add(oneTime, lineGross)
		}
		if attrs.PaymentTiming == pricing.Prepaid {
			firstInvoice.Add(firstInvoice, lineGross)
		}

		discount := ""
		if strings.TrimSpace(l.DiscountPercent) != "" {
			discount = discPct.FloatString(2)
		}
		items = append(items, repository.QuotationItemInput{
			ProductID: l.ProductID, Description: description, SKU: strings.TrimSpace(l.SKU), Unit: strings.TrimSpace(l.Unit),
			Quantity: qty.FloatString(2), UnitPrice: price.FloatString(2), DiscountPercent: discount,
			TaxPercent: taxPct.FloatString(2), TaxAmount: tax.FloatString(2), LineTotal: net.FloatString(2), Position: i, Pricing: attrs,
		})
	}
	if !anyLineTax && strings.TrimSpace(legacyTaxTotal) != "" {
		legacy, ok := parseNonNegative(legacyTaxTotal, "0", nil)
		if !ok {
			return QuotationTotals{}, nil, ErrInvalidQuotationAmount
		}
		taxTotal = legacy
	}
	grand := new(big.Rat).Add(new(big.Rat).Sub(subtotal, discountTotal), taxTotal)
	recurringTotals := make(map[pricing.Frequency]string, len(recurring))
	for f, v := range recurring {
		recurringTotals[f] = v.FloatString(2)
	}
	return QuotationTotals{
		Subtotal: subtotal.FloatString(2), DiscountTotal: discountTotal.FloatString(2),
		TaxTotal: taxTotal.FloatString(2), GrandTotal: grand.FloatString(2),
		OneTimeTotal: oneTime.FloatString(2), FirstInvoiceTotal: firstInvoice.FloatString(2), RecurringTotals: recurringTotals,
	}, items, nil
}
