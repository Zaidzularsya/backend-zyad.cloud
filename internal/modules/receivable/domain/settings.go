package domain

type Settings struct {
	InvoiceLeadDays, PaymentTermsDays int
	DefaultChannels                   []string
	DefaultSenderUserID               string
}
