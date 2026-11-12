package domain

type Settings struct {
	InvoiceLeadDays, PaymentTermsDays int
	DefaultChannels                   []string
	DefaultSenderUserID               string
	// DefaultBillingDay (1–31) prefills "tanggal tagih setiap bulan" when
	// onboarding a customer; nil = use the onboarding day.
	DefaultBillingDay *int
}
