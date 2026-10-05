// Package pricing menyimpan kosakata atribut harga (jenis tagihan, frekuensi,
// waktu pembayaran) yang dipakai bersama oleh catalog, crm, dan receivable.
package pricing

import "errors"

type ChargeType string
type Frequency string
type PaymentTiming string

const (
	OneTime   ChargeType = "one_time"
	Recurring ChargeType = "recurring"

	Daily      Frequency = "daily"
	Weekly     Frequency = "weekly"
	Monthly    Frequency = "monthly"
	Quarterly  Frequency = "quarterly"
	Semiannual Frequency = "semiannual"
	Annual     Frequency = "annual"

	Prepaid  PaymentTiming = "prepaid"
	Postpaid PaymentTiming = "postpaid"
)

// Attributes adalah atribut harga satu produk atau satu baris quotation.
type Attributes struct {
	ChargeType    ChargeType
	Frequency     Frequency
	PaymentTiming PaymentTiming
}

var ErrInvalidAttributes = errors.New("charge_type must be one_time|recurring, billing_frequency required only for recurring (daily|weekly|monthly|quarterly|semiannual|annual), payment_timing must be prepaid|postpaid")

var frequencies = []Frequency{Daily, Weekly, Monthly, Quarterly, Semiannual, Annual}

var frequencyLabels = map[Frequency][2]string{
	Daily:      {"Harian", "/hari"},
	Weekly:     {"Mingguan", "/minggu"},
	Monthly:    {"Bulanan", "/bulan"},
	Quarterly:  {"Triwulan", "/triwulan"},
	Semiannual: {"Semesteran", "/semester"},
	Annual:     {"Tahunan", "/tahun"},
}

// Normalize mengisi default (one_time, prepaid) untuk nilai kosong lalu memvalidasi.
func Normalize(a Attributes) (Attributes, error) {
	if a.ChargeType == "" {
		a.ChargeType = OneTime
	}
	if a.PaymentTiming == "" {
		a.PaymentTiming = Prepaid
	}
	switch a.ChargeType {
	case OneTime:
		if a.Frequency != "" {
			return Attributes{}, ErrInvalidAttributes
		}
	case Recurring:
		if _, ok := frequencyLabels[a.Frequency]; !ok {
			return Attributes{}, ErrInvalidAttributes
		}
	default:
		return Attributes{}, ErrInvalidAttributes
	}
	if a.PaymentTiming != Prepaid && a.PaymentTiming != Postpaid {
		return Attributes{}, ErrInvalidAttributes
	}
	return a, nil
}

// Frequencies mengembalikan frekuensi dalam urutan tetap daily→annual.
func Frequencies() []Frequency {
	return append([]Frequency(nil), frequencies...)
}

func (f Frequency) Label() string       { return frequencyLabels[f][0] }
func (f Frequency) PriceSuffix() string { return frequencyLabels[f][1] }

func (c ChargeType) Label() string {
	if c == Recurring {
		return "Berulang"
	}
	return "Sekali bayar"
}

func (p PaymentTiming) Label() string {
	if p == Postpaid {
		return "Pascabayar"
	}
	return "Prabayar"
}

// Short meringkas atribut untuk badge/PDF: frekuensi menggantikan "Berulang".
func (a Attributes) Short() string {
	lead := a.ChargeType.Label()
	if a.ChargeType == Recurring {
		lead = a.Frequency.Label()
	}
	return lead + " · " + a.PaymentTiming.Label()
}
