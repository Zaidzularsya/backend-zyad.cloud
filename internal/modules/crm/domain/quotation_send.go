package domain

import "time"

type QuotationSendChannel string

type QuotationSendMode string

type QuotationSendStatus string

const (
	QuotationChannelEmail    QuotationSendChannel = "email"
	QuotationChannelWhatsApp QuotationSendChannel = "whatsapp"

	QuotationModeText    QuotationSendMode = "text"
	QuotationModePDF     QuotationSendMode = "pdf"
	QuotationModeTextPDF QuotationSendMode = "text_pdf"

	QuotationSendSent   QuotationSendStatus = "sent"
	QuotationSendFailed QuotationSendStatus = "failed"
)

// NeedsPDF: mode pdf atau text_pdf melampirkan snapshot PDF final.
func (m QuotationSendMode) NeedsPDF() bool {
	return m == QuotationModePDF || m == QuotationModeTextPDF
}

// NeedsText: mode text atau text_pdf menyertakan ringkasan teks.
func (m QuotationSendMode) NeedsText() bool {
	return m == QuotationModeText || m == QuotationModeTextPDF
}

// QuotationSend adalah satu percobaan kirim penawaran (sukses atau gagal).
type QuotationSend struct {
	ID                string
	QuotationID       string
	Channel           QuotationSendChannel
	Mode              QuotationSendMode
	Recipient         string
	PDFAssetID        *string
	Status            QuotationSendStatus
	Error             string
	ExternalMessageID string
	ClientRequestID   string
	SentBy            string
	SentByName        string // join users, read-only
	SentAt            time.Time
}
