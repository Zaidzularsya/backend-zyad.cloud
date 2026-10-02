package service

import (
	"errors"
	"math/big"
	"testing"
)

func TestRoundHalfUp2(t *testing.T) {
	cases := map[string]string{"1.005": "1.01", "1.004": "1.00", "2.675": "2.68", "0": "0.00", "99999.995": "100000.00"}
	for in, want := range cases {
		r, _ := new(big.Rat).SetString(in)
		if got := roundHalfUp2(r).FloatString(2); got != want {
			t.Errorf("roundHalfUp2(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestPriceQuotationLinesPerLineRounding(t *testing.T) {
	totals, items, err := priceQuotationLines([]QuotationLineInput{
		{Description: "Router", Quantity: "3", UnitPrice: "33333.33", DiscountPercent: "10", TaxPercent: "11", Unit: "pcs"},
		{Description: "Instalasi", Quantity: "1", UnitPrice: "500000"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	a := items[0]
	if a.LineTotal != "89999.99" || a.TaxAmount != "9900.00" || a.Quantity != "3.00" || a.UnitPrice != "33333.33" || a.Unit != "pcs" || a.Position != 0 {
		t.Fatalf("line A = %+v", a)
	}
	if totals != (QuotationTotals{Subtotal: "599999.99", DiscountTotal: "10000.00", TaxTotal: "9900.00", GrandTotal: "599899.99"}) {
		t.Fatalf("totals = %+v", totals)
	}
}

func TestPriceQuotationLinesLegacyHeaderTax(t *testing.T) {
	// Kontrak lama: tanpa pajak per baris, tax_total header dipakai apa adanya.
	totals, items, err := priceQuotationLines([]QuotationLineInput{
		{Description: "Item A", Quantity: "2", UnitPrice: "150000", DiscountPercent: "10"},
		{Description: "Item B", Quantity: "1", UnitPrice: "99999.99"},
	}, "5000.01")
	if err != nil {
		t.Fatal(err)
	}
	if totals.Subtotal != "399999.99" || totals.DiscountTotal != "30000.00" || totals.TaxTotal != "5000.01" || totals.GrandTotal != "375000.00" {
		t.Fatalf("totals = %+v", totals)
	}
	if items[0].LineTotal != "270000.00" || items[1].LineTotal != "99999.99" {
		t.Fatalf("items = %+v", items)
	}
	// Pajak per baris mengalahkan header.
	totals, _, _ = priceQuotationLines([]QuotationLineInput{{Description: "X", Quantity: "1", UnitPrice: "100", TaxPercent: "11"}}, "5000")
	if totals.TaxTotal != "11.00" {
		t.Fatalf("line tax must win, got %s", totals.TaxTotal)
	}
}

func TestPriceQuotationLinesRecurringDiscount(t *testing.T) {
	totals, _, err := priceQuotationLines([]QuotationLineInput{{Description: "A", Quantity: "1", UnitPrice: "300000", DiscountPercent: "33.33"}}, "")
	if err != nil || totals.DiscountTotal != "99990.00" {
		t.Fatalf("discount = %s err = %v", totals.DiscountTotal, err)
	}
}

func TestPriceQuotationLinesRejectsInvalid(t *testing.T) {
	for _, l := range []QuotationLineInput{
		{Description: "A", Quantity: "-1", UnitPrice: "1"},
		{Description: "A", Quantity: "1", UnitPrice: "abc"},
		{Description: "A", Quantity: "1", UnitPrice: "1", DiscountPercent: "101"},
		{Description: "A", Quantity: "1", UnitPrice: "1", TaxPercent: "100.5"},
		{Description: " ", Quantity: "1", UnitPrice: "1"},
	} {
		if _, _, err := priceQuotationLines([]QuotationLineInput{l}, ""); !errors.Is(err, ErrInvalidQuotationAmount) {
			t.Errorf("%+v: err = %v", l, err)
		}
	}
	if _, _, err := priceQuotationLines(nil, ""); !errors.Is(err, ErrInvalidQuotationAmount) {
		t.Error("empty lines must be rejected")
	}
}
