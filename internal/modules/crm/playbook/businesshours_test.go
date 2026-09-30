package playbook

import (
	"testing"
	"time"
)

func wib(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, BusinessLocation)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAddBusinessMinutes(t *testing.T) {
	// 2026-10-02 = Jumat, 2026-10-03 = Sabtu.
	cases := []struct{ name, in, want string }{
		{"dalam jam kerja", "2026-09-30 10:00", "2026-09-30 11:00"},
		{"lintas sore", "2026-09-30 16:30", "2026-10-01 08:30"},
		{"jumat sore ke senin", "2026-10-02 16:30", "2026-10-05 08:30"},
		{"sabtu", "2026-10-03 10:00", "2026-10-05 09:00"},
		{"malam hari kerja", "2026-09-29 22:00", "2026-09-30 09:00"},
		{"subuh", "2026-09-30 05:00", "2026-09-30 09:00"},
		{"tepat jam 17", "2026-09-30 17:00", "2026-10-01 09:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AddBusinessMinutes(wib(c.in), 60)
			if !got.Equal(wib(c.want)) {
				t.Fatalf("got %s want %s", got.In(BusinessLocation), c.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("result must be UTC, got %s", got.Location())
			}
		})
	}
}

func TestBusinessDaysAt(t *testing.T) {
	cases := []struct {
		name, in string
		days     int
		want     string
	}{
		{"rabu +1", "2026-09-30 15:00", 1, "2026-10-01 09:00"},
		{"jumat +1 ke senin", "2026-10-02 10:00", 1, "2026-10-05 09:00"},
		{"rabu +3", "2026-09-30 10:00", 3, "2026-10-05 09:00"},
		{"sabtu +1", "2026-10-03 10:00", 1, "2026-10-05 09:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BusinessDaysAt(wib(c.in), c.days, "09:00")
			if !got.Equal(wib(c.want)) {
				t.Fatalf("got %s want %s", got.In(BusinessLocation), c.want)
			}
		})
	}
}
