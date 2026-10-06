package pricing

import (
	"errors"
	"math/big"
	"strings"
	"unicode/utf8"
)

// LineInput adalah satu baris mentah dari pemanggil. Nilai kosong memakai
// default: qty 1, harga 0, diskon/pajak 0, atribut one_time + prepaid.
type LineInput struct {
	Description, Quantity, UnitPrice, DiscountPercent, TaxPercent string
	Attrs                                                         Attributes
}

// PricedLine adalah hasil hitung satu baris. DiscountPercent kosong bila
// pemanggil tidak mengirim diskon (membedakan "tanpa diskon" dari "0%").
type PricedLine struct {
	Description, Quantity, UnitPrice, DiscountPercent, TaxPercent, TaxAmount, LineTotal string
	Attrs                                                                               Attributes
}

// Totals memuat total header. Rincian: total per baris = line_total + pajak;
// FirstInvoiceTotal = semua baris prabayar (sekali bayar maupun berulang periode pertama).
type Totals struct {
	Subtotal, DiscountTotal, TaxTotal, GrandTotal string
	OneTimeTotal, FirstInvoiceTotal               string
	RecurringTotals                               map[Frequency]string
}

var ErrInvalidLine = errors.New("description 1-500 chars; quantity/price >= 0; discount/tax 0-100; valid pricing attributes")

var (
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
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok || r.Sign() < 0 || (max != nil && r.Cmp(max) > 0) {
		return nil, false
	}
	return r, true
}

// PriceLines menghitung setiap baris (dibulatkan setengah ke atas per baris)
// dan total header. Tidak menangani tax_total header legacy; itu urusan CRM.
func PriceLines(lines []LineInput) (Totals, []PricedLine, error) {
	if len(lines) == 0 {
		return Totals{}, nil, ErrInvalidLine
	}
	subtotal, discountTotal, taxTotal := new(big.Rat), new(big.Rat), new(big.Rat)
	oneTime, firstInvoice := new(big.Rat), new(big.Rat)
	recurring := map[Frequency]*big.Rat{}
	priced := make([]PricedLine, 0, len(lines))
	for _, l := range lines {
		description := strings.TrimSpace(l.Description)
		qty, ok1 := parseNonNegative(l.Quantity, "1", nil)
		price, ok2 := parseNonNegative(l.UnitPrice, "0", nil)
		discPct, ok3 := parseNonNegative(l.DiscountPercent, "0", ratHundred)
		taxPct, ok4 := parseNonNegative(l.TaxPercent, "0", ratHundred)
		attrs, attrErr := Normalize(l.Attrs)
		if attrErr != nil || description == "" || utf8.RuneCountInString(description) > 500 || !ok1 || !ok2 || !ok3 || !ok4 {
			return Totals{}, nil, ErrInvalidLine
		}
		gross := roundHalfUp2(new(big.Rat).Mul(qty, price))
		disc := roundHalfUp2(new(big.Rat).Quo(new(big.Rat).Mul(gross, discPct), ratHundred))
		net := new(big.Rat).Sub(gross, disc)
		tax := roundHalfUp2(new(big.Rat).Quo(new(big.Rat).Mul(net, taxPct), ratHundred))
		subtotal.Add(subtotal, gross)
		discountTotal.Add(discountTotal, disc)
		taxTotal.Add(taxTotal, tax)

		lineGross := new(big.Rat).Add(net, tax)
		if attrs.ChargeType == Recurring {
			if recurring[attrs.Frequency] == nil {
				recurring[attrs.Frequency] = new(big.Rat)
			}
			recurring[attrs.Frequency].Add(recurring[attrs.Frequency], lineGross)
		} else {
			oneTime.Add(oneTime, lineGross)
		}
		if attrs.PaymentTiming == Prepaid {
			firstInvoice.Add(firstInvoice, lineGross)
		}

		discount := ""
		if strings.TrimSpace(l.DiscountPercent) != "" {
			discount = discPct.FloatString(2)
		}
		priced = append(priced, PricedLine{
			Description: description, Quantity: qty.FloatString(2), UnitPrice: price.FloatString(2),
			DiscountPercent: discount, TaxPercent: taxPct.FloatString(2), TaxAmount: tax.FloatString(2),
			LineTotal: net.FloatString(2), Attrs: attrs,
		})
	}
	return buildTotals(subtotal, discountTotal, taxTotal, oneTime, firstInvoice, recurring), priced, nil
}

// WithTaxTotal menyusun ulang Totals dari totals yang ada dengan total pajak diganti
// (dipakai CRM untuk kontrak lama tax_total header).
func (t Totals) WithTaxTotal(tax string) (Totals, bool) {
	taxR, ok := new(big.Rat).SetString(tax)
	sub, ok2 := new(big.Rat).SetString(t.Subtotal)
	disc, ok3 := new(big.Rat).SetString(t.DiscountTotal)
	if !ok || !ok2 || !ok3 {
		return t, false
	}
	t.TaxTotal = taxR.FloatString(2)
	t.GrandTotal = new(big.Rat).Add(new(big.Rat).Sub(sub, disc), taxR).FloatString(2)
	return t, true
}

func buildTotals(subtotal, discountTotal, taxTotal, oneTime, firstInvoice *big.Rat, recurring map[Frequency]*big.Rat) Totals {
	grand := new(big.Rat).Add(new(big.Rat).Sub(subtotal, discountTotal), taxTotal)
	recurringTotals := make(map[Frequency]string, len(recurring))
	for f, v := range recurring {
		recurringTotals[f] = v.FloatString(2)
	}
	return Totals{
		Subtotal: subtotal.FloatString(2), DiscountTotal: discountTotal.FloatString(2),
		TaxTotal: taxTotal.FloatString(2), GrandTotal: grand.FloatString(2),
		OneTimeTotal: oneTime.FloatString(2), FirstInvoiceTotal: firstInvoice.FloatString(2), RecurringTotals: recurringTotals,
	}
}
