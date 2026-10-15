package domain

import "time"

type Payment struct {
	ID, InvoiceID, Amount, Method, Reference, Note, RecordedBy string
	// InvoiceNumber & AccountName hanya terisi oleh PaymentRepository.ListAll (join baca).
	InvoiceNumber, AccountName string
	PaidAt, CreatedAt          time.Time
}
