package service

import (
	"errors"
	"testing"

	"zyad.cloud/internal/shared/pricing"
)

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
	if totals.Subtotal != "599999.99" || totals.DiscountTotal != "10000.00" || totals.TaxTotal != "9900.00" || totals.GrandTotal != "599899.99" {
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

func TestPriceQuotationLinesBreakdown(t *testing.T) {
	lines := []QuotationLineInput{
		{Description: "Instalasi", Quantity: "1", UnitPrice: "500000", Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}},
		{Description: "Internet 50 Mbps", Quantity: "1", UnitPrice: "300000", TaxPercent: "11", Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}},
		{Description: "Website", Quantity: "1", UnitPrice: "5000000", Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Postpaid}},
		{Description: "Domain", Quantity: "1", UnitPrice: "200000", Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Annual, PaymentTiming: pricing.Postpaid}},
		{Description: "Baris bebas", Quantity: "1", UnitPrice: "1000"}, // default one_time prepaid
	}
	totals, items, err := priceQuotationLines(lines, "")
	if err != nil {
		t.Fatal(err)
	}
	if totals.OneTimeTotal != "5501000.00" || totals.FirstInvoiceTotal != "834000.00" {
		t.Fatalf("one_time=%s first=%s", totals.OneTimeTotal, totals.FirstInvoiceTotal)
	}
	if totals.RecurringTotals[pricing.Monthly] != "333000.00" || totals.RecurringTotals[pricing.Annual] != "200000.00" || len(totals.RecurringTotals) != 2 {
		t.Fatalf("recurring=%v", totals.RecurringTotals)
	}
	if totals.GrandTotal != "6034000.00" {
		t.Fatalf("grand=%s", totals.GrandTotal)
	}
	if items[4].Pricing != (pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}) {
		t.Fatalf("default item pricing %+v", items[4].Pricing)
	}
	if _, _, err := priceQuotationLines([]QuotationLineInput{{Description: "x", Pricing: pricing.Attributes{ChargeType: pricing.Recurring}}}, ""); !errors.Is(err, ErrInvalidQuotationAmount) {
		t.Fatalf("invalid pricing err=%v", err)
	}
	only, _, _ := priceQuotationLines([]QuotationLineInput{{Description: "A", UnitPrice: "10"}}, "")
	if only.RecurringTotals == nil || len(only.RecurringTotals) != 0 {
		t.Fatalf("recurring harus map kosong non-nil: %#v", only.RecurringTotals)
	}
}
