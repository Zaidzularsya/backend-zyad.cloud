package docpdf

import (
	"testing"
	"time"
)

func TestFormatters(t *testing.T) {
	cases := []struct {
		fn       func(string) string
		in, want string
	}{
		{FormatRupiah, "1500000.00", "Rp 1.500.000"},
		{FormatRupiah, "1500000.50", "Rp 1.500.000,50"},
		{FormatRupiah, "0.00", "Rp 0"},
		{FormatRupiah, "999.99", "Rp 999,99"},
		{FormatQuantity, "2.00", "2"},
		{FormatQuantity, "1.50", "1,5"},
		{FormatPercent, "11.00", "11%"},
		{FormatPercent, "12.50", "12,5%"},
		{FormatPercent, "0.00", "-"},
		{FormatPercent, "", "-"},
	}
	for _, c := range cases {
		if got := c.fn(c.in); got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
	if got := FormatDateID(time.Date(2026, 11, 29, 20, 0, 0, 0, time.UTC)); got != "30 Nov 2026" {
		t.Errorf("FormatDateID = %q (harus tanggal Asia/Jakarta)", got)
	}
}
