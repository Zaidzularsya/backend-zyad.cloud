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
