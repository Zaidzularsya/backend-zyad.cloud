package domain

import "time"

const SourceTypeCRMContact = "crm_contact"

// Account adalah pihak yang ditagih. SourceType/SourceID kosong untuk account manual.
type Account struct {
	ID, Name, CompanyName, Email, Phone, Address string
	SourceType, SourceID                         string // "crm_contact", <contact id> atau kosong
	CreatedAt, UpdatedAt                         time.Time
}

// ContactID mengembalikan id kontak CRM bila account berasal dari kontak.
func (a Account) ContactID() string {
	if a.SourceType == SourceTypeCRMContact {
		return a.SourceID
	}
	return ""
}
