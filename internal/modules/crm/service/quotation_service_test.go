package service

import "testing"

func TestFormatDocumentNumber(t *testing.T) {
	got := formatDocumentNumber("QUO", 2026, 1)
	want := "QUO-2026-0001"
	if got != want {
		t.Fatalf("formatDocumentNumber() = %q, want %q", got, want)
	}

	got = formatDocumentNumber("INV", 2026, 12345)
	want = "INV-2026-12345"
	if got != want {
		t.Fatalf("formatDocumentNumber() = %q, want %q", got, want)
	}
}

func TestQuotationComputeTotals(t *testing.T) {
	svc := &quotationService{}

	items := []QuotationLineInput{
		{Description: "Item A", Quantity: "2", UnitPrice: "150000", DiscountPercent: "10"},
		{Description: "Item B", Quantity: "1", UnitPrice: "99999.99"},
	}

	subtotal, discountTotal, grandTotal, lineItems, err := svc.computeTotals(items, "5000.01")
	if err != nil {
		t.Fatalf("computeTotals() error = %v", err)
	}

	if subtotal != "399999.99" {
		t.Errorf("subtotal = %q, want %q", subtotal, "399999.99")
	}
	if discountTotal != "30000.00" {
		t.Errorf("discountTotal = %q, want %q", discountTotal, "30000.00")
	}
	if grandTotal != "375000.00" {
		t.Errorf("grandTotal = %q, want %q", grandTotal, "375000.00")
	}
	if len(lineItems) != 2 {
		t.Fatalf("len(lineItems) = %d, want 2", len(lineItems))
	}
	if lineItems[0].LineTotal != "270000.00" {
		t.Errorf("lineItems[0].LineTotal = %q, want %q", lineItems[0].LineTotal, "270000.00")
	}
	if lineItems[1].LineTotal != "99999.99" {
		t.Errorf("lineItems[1].LineTotal = %q, want %q", lineItems[1].LineTotal, "99999.99")
	}
}

func TestQuotationComputeTotalsRecurringDiscount(t *testing.T) {
	// 33.33% of 300000 is not a terminating binary fraction the way
	// float64 would round it — exercised here to confirm math/big.Rat
	// division rounds to the same 2dp result a human would expect.
	svc := &quotationService{}

	items := []QuotationLineInput{
		{Description: "Item A", Quantity: "1", UnitPrice: "300000", DiscountPercent: "33.33"},
	}

	_, discountTotal, _, _, err := svc.computeTotals(items, "0")
	if err != nil {
		t.Fatalf("computeTotals() error = %v", err)
	}
	if discountTotal != "99990.00" {
		t.Errorf("discountTotal = %q, want %q", discountTotal, "99990.00")
	}
}

func TestInvoiceComputeInvoiceTotals(t *testing.T) {
	svc := &invoiceService{}

	items := []InvoiceLineInput{
		{Description: "Item A", Quantity: "3", UnitPrice: "10000", DiscountPercent: "5"},
	}

	subtotal, grandTotal, lineItems, err := svc.computeInvoiceTotals(items, "1425")
	if err != nil {
		t.Fatalf("computeInvoiceTotals() error = %v", err)
	}

	if subtotal != "30000.00" {
		t.Errorf("subtotal = %q, want %q", subtotal, "30000.00")
	}
	// lineTotal = 30000 - 5% = 28500.00; grandTotal = 28500 + 1425 = 29925.00
	if grandTotal != "29925.00" {
		t.Errorf("grandTotal = %q, want %q", grandTotal, "29925.00")
	}
	if len(lineItems) != 1 || lineItems[0].LineTotal != "28500.00" {
		t.Fatalf("lineItems = %+v, want single item with LineTotal 28500.00", lineItems)
	}
}
