package pricing

import "time"

// AddPeriod: tanggal kalender (jam diabaikan, hasil UTC midnight). Bulanan/triwulan/semester/tahunan = start + n×(1/3/6/12) bulan
// dihitung dari start (bukan berantai) dengan clamp ke hari terakhir bulan. Harian/mingguan = +n / +7n hari.
func AddPeriod(start time.Time, f Frequency, n int) time.Time {
	y, m, d := start.Date()
	base := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	switch f {
	case Daily:
		return base.AddDate(0, 0, n)
	case Weekly:
		return base.AddDate(0, 0, 7*n)
	}
	months := map[Frequency]int{Monthly: 1, Quarterly: 3, Semiannual: 6, Annual: 12}[f]
	total := int(m) - 1 + months*n
	ty, tm := y+total/12, time.Month(total%12+1)
	if total < 0 && total%12 != 0 {
		ty, tm = y+total/12-1, time.Month(total%12+13)
	}
	last := time.Date(ty, tm+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(ty, tm, min(d, last), 0, 0, 0, 0, time.UTC)
}

// PeriodRange: [AddPeriod(n), AddPeriod(n+1) − 1 hari].
func PeriodRange(start time.Time, f Frequency, n int) (from, to time.Time) {
	return AddPeriod(start, f, n), AddPeriod(start, f, n+1).AddDate(0, 0, -1)
}
