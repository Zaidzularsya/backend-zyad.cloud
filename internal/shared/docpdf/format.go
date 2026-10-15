package docpdf

import (
	"math/big"
	"strings"
	"time"

	"zyad.cloud/internal/core/businesstime"
)

var monthsID = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func FormatDateID(t time.Time) string {
	d := t.In(businesstime.Location())
	return strings.Join([]string{itoa(d.Day()), monthsID[d.Month()-1], itoa(d.Year())}, " ")
}

func itoa(n int) string { return big.NewInt(int64(n)).String() }

// groupThousands: "1500000" → "1.500.000".
func groupThousands(digits string) string {
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// splitDecimal: "1500000.50" → ("1500000", "50"); pecahan nol dihapus dari kanan.
func splitDecimal(v string) (string, string) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(v))
	if !ok {
		return "0", ""
	}
	s := r.FloatString(2)
	whole, frac, _ := strings.Cut(s, ".")
	return strings.TrimPrefix(whole, "-"), strings.TrimRight(frac, "0")
}

func FormatRupiah(v string) string {
	whole, frac := splitDecimal(v)
	out := "Rp " + groupThousands(whole)
	if frac != "" {
		if len(frac) == 1 {
			frac += "0"
		}
		out += "," + frac
	}
	return out
}

func FormatQuantity(v string) string {
	whole, frac := splitDecimal(v)
	if frac == "" {
		return groupThousands(whole)
	}
	return groupThousands(whole) + "," + frac
}

func FormatPercent(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	whole, frac := splitDecimal(v)
	if whole == "0" && frac == "" {
		return "-"
	}
	if frac == "" {
		return whole + "%"
	}
	return whole + "," + frac + "%"
}
