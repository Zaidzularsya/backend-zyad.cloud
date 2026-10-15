package pricing

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

func TestPriceLinesPerLineRounding(t *testing.T) {
	totals, lines, err := PriceLines([]LineInput{
		{Description: "Router", Quantity: "3", UnitPrice: "33333.33", DiscountPercent: "10", TaxPercent: "11"},
		{Description: "Instalasi", Quantity: "1", UnitPrice: "500000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := lines[0]
	if a.LineTotal != "89999.99" || a.TaxAmount != "9900.00" || a.Quantity != "3.00" || a.UnitPrice != "33333.33" || a.DiscountPercent != "10.00" {
		t.Fatalf("line A = %+v", a)
	}
	if lines[1].DiscountPercent != "" {
		t.Fatalf("discount not sent must stay empty, got %q", lines[1].DiscountPercent)
	}
	if totals.Subtotal != "599999.99" || totals.DiscountTotal != "10000.00" || totals.TaxTotal != "9900.00" || totals.GrandTotal != "599899.99" {
		t.Fatalf("totals = %+v", totals)
	}
}

func TestPriceLinesRejectsInvalid(t *testing.T) {
	for _, l := range []LineInput{
		{Description: "A", Quantity: "-1", UnitPrice: "1"},
		{Description: "A", Quantity: "1", UnitPrice: "abc"},
		{Description: "A", Quantity: "1", UnitPrice: "1", DiscountPercent: "101"},
		{Description: "A", Quantity: "1", UnitPrice: "1", TaxPercent: "100.5"},
		{Description: " ", Quantity: "1", UnitPrice: "1"},
		{Description: "x", Attrs: Attributes{ChargeType: Recurring}},
	} {
		if _, _, err := PriceLines([]LineInput{l}); !errors.Is(err, ErrInvalidLine) {
			t.Errorf("%+v: err = %v", l, err)
		}
	}
	if _, _, err := PriceLines(nil); !errors.Is(err, ErrInvalidLine) {
		t.Error("empty lines must be rejected")
	}
}

func TestPriceLinesBreakdown(t *testing.T) {
	totals, lines, err := PriceLines([]LineInput{
		{Description: "Instalasi", Quantity: "1", UnitPrice: "500000", Attrs: Attributes{ChargeType: OneTime, PaymentTiming: Prepaid}},
		{Description: "Internet 50 Mbps", Quantity: "1", UnitPrice: "300000", TaxPercent: "11", Attrs: Attributes{ChargeType: Recurring, Frequency: Monthly, PaymentTiming: Prepaid}},
		{Description: "Website", Quantity: "1", UnitPrice: "5000000", Attrs: Attributes{ChargeType: OneTime, PaymentTiming: Postpaid}},
		{Description: "Domain", Quantity: "1", UnitPrice: "200000", Attrs: Attributes{ChargeType: Recurring, Frequency: Annual, PaymentTiming: Postpaid}},
		{Description: "Baris bebas", Quantity: "1", UnitPrice: "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if totals.OneTimeTotal != "5501000.00" || totals.FirstInvoiceTotal != "834000.00" {
		t.Fatalf("one_time=%s first=%s", totals.OneTimeTotal, totals.FirstInvoiceTotal)
	}
	if totals.RecurringTotals[Monthly] != "333000.00" || totals.RecurringTotals[Annual] != "200000.00" || len(totals.RecurringTotals) != 2 {
		t.Fatalf("recurring=%v", totals.RecurringTotals)
	}
	if totals.GrandTotal != "6034000.00" {
		t.Fatalf("grand=%s", totals.GrandTotal)
	}
	if lines[4].Attrs != (Attributes{ChargeType: OneTime, PaymentTiming: Prepaid}) {
		t.Fatalf("default attrs %+v", lines[4].Attrs)
	}
	only, _, _ := PriceLines([]LineInput{{Description: "A", UnitPrice: "10"}})
	if only.RecurringTotals == nil || len(only.RecurringTotals) != 0 {
		t.Fatalf("recurring harus map kosong non-nil: %#v", only.RecurringTotals)
	}
}
