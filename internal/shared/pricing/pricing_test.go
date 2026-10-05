package pricing

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	got, err := Normalize(Attributes{})
	if err != nil || got != (Attributes{ChargeType: OneTime, PaymentTiming: Prepaid}) {
		t.Fatalf("default = %+v err=%v", got, err)
	}
	for _, bad := range []Attributes{
		{ChargeType: Recurring},                   // tanpa frekuensi
		{ChargeType: OneTime, Frequency: Monthly}, // one_time dengan frekuensi
		{ChargeType: "usage"},                     // ditahan (K2)
		{ChargeType: Recurring, Frequency: "biweekly"},
		{PaymentTiming: "cod"},
	} {
		if _, err := Normalize(bad); !errors.Is(err, ErrInvalidAttributes) {
			t.Errorf("%+v err=%v", bad, err)
		}
	}
	ok, _ := Normalize(Attributes{ChargeType: Recurring, Frequency: Monthly, PaymentTiming: Postpaid})
	if ok.Short() != "Bulanan · Pascabayar" || Monthly.PriceSuffix() != "/bulan" || Semiannual.Label() != "Semesteran" {
		t.Fatalf("labels: %q %q %q", ok.Short(), Monthly.PriceSuffix(), Semiannual.Label())
	}
	if (Attributes{ChargeType: OneTime, PaymentTiming: Prepaid}).Short() != "Sekali bayar · Prabayar" {
		t.Fatal("one_time short")
	}
	if f := Frequencies(); len(f) != 6 || f[0] != Daily || f[5] != Annual {
		t.Fatalf("order %v", f)
	}
}
