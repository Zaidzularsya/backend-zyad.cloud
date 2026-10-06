package pricing

import (
	"testing"
	"time"
)

func TestAddPeriod(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	cases := []struct {
		start string
		f     Frequency
		n     int
		want  string
	}{
		{"2026-01-31", Monthly, 1, "2026-02-28"},
		{"2026-01-31", Monthly, 2, "2026-03-31"},
		{"2028-01-31", Monthly, 1, "2028-02-29"},
		{"2026-11-30", Quarterly, 1, "2027-02-28"},
		{"2026-08-31", Semiannual, 1, "2027-02-28"},
		{"2028-02-29", Annual, 1, "2029-02-28"},
		{"2028-02-29", Annual, 4, "2032-02-29"},
		{"2026-10-05", Weekly, 2, "2026-10-19"},
		{"2026-12-31", Daily, 1, "2027-01-01"},
		{"2026-10-05", Monthly, 0, "2026-10-05"},
		{"2026-01-15", Monthly, 24, "2028-01-15"},
	}
	for _, c := range cases {
		if got := AddPeriod(d(c.start), c.f, c.n).Format("2006-01-02"); got != c.want {
			t.Errorf("%s %s +%d = %s, want %s", c.start, c.f, c.n, got, c.want)
		}
	}
	from, to := PeriodRange(d("2026-01-31"), Monthly, 1)
	if from.Format("2006-01-02") != "2026-02-28" || to.Format("2006-01-02") != "2026-03-30" {
		t.Fatalf("range %v–%v", from, to)
	}
}
