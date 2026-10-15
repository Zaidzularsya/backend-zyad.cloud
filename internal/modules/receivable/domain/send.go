package domain

import "time"

type Send struct {
	ID, InvoiceID, Channel, Recipient, Status, Error, ExternalMessageID, ClientRequestID, Trigger, SentBy, SentByName string
	SentAt                                                                                                            time.Time
}
