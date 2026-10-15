package dto

import (
	"zyad.cloud/internal/modules/receivable/domain"
)

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func AccountFromDomain(a domain.Account) AccountResponse {
	return AccountResponse{
		ID: a.ID, Name: a.Name, CompanyName: a.CompanyName, Email: a.Email, Phone: a.Phone, Address: a.Address,
		ContactID: strPtr(a.ContactID()), CreatedAt: a.CreatedAt,
	}
}

func SendFromDomain(s domain.Send) InvoiceSendResponse {
	return InvoiceSendResponse{
		ID: s.ID, InvoiceID: s.InvoiceID, Channel: s.Channel, Recipient: s.Recipient, Status: s.Status, Error: s.Error,
		Trigger: s.Trigger, SentBy: strPtr(s.SentBy), SentByName: s.SentByName, SentAt: s.SentAt,
	}
}

// InvoiceFromDomain: last (opsional) mengisi last_send. Item selalu array (kosong pada daftar).
func InvoiceFromDomain(inv domain.Invoice, last *domain.Send) InvoiceResponse {
	out := InvoiceResponse{
		ID: inv.ID, InvoiceNumber: strPtr(inv.InvoiceNumber), Status: string(inv.Status), Account: AccountFromDomain(inv.Account),
		SourceType: string(inv.SourceType), SourceID: strPtr(inv.SourceID), ContractID: strPtr(inv.ContractID),
		PeriodStart: DateString(inv.PeriodStart), PeriodEnd: DateString(inv.PeriodEnd),
		IssueDate: DateString(inv.IssueDate), DueDate: DateString(inv.DueDate), Currency: inv.Currency,
		Subtotal: inv.Subtotal, DiscountTotal: inv.DiscountTotal, TaxTotal: inv.TaxTotal, GrandTotal: inv.GrandTotal,
		AmountPaid: inv.AmountPaid, Balance: inv.Balance(), Channels: inv.Channels, PICUserID: strPtr(inv.PICUserID),
		Notes: inv.Notes, VoidReason: inv.VoidReason, PaidAt: inv.PaidAt, VoidedAt: inv.VoidedAt,
		Items: []InvoiceItemResponse{}, CreatedAt: inv.CreatedAt,
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}
	for _, it := range inv.Items {
		out.Items = append(out.Items, InvoiceItemResponse{
			ID: it.ID, Description: it.Description, Quantity: it.Quantity, Unit: it.Unit, UnitPrice: it.UnitPrice,
			DiscountPercent: strPtr(it.DiscountPercent), TaxPercent: it.TaxPercent, TaxAmount: it.TaxAmount, LineTotal: it.LineTotal,
			ProductID: strPtr(it.ProductID), SKU: it.SKU, ChargeType: string(it.Pricing.ChargeType),
			BillingFrequency: strPtr(string(it.Pricing.Frequency)), PaymentTiming: string(it.Pricing.PaymentTiming),
			PeriodStart: DateString(it.PeriodStart), PeriodEnd: DateString(it.PeriodEnd), Position: it.Position,
		})
	}
	if last != nil {
		s := SendFromDomain(*last)
		out.LastSend = &s
	}
	return out
}

func PaymentFromDomain(p domain.Payment) PaymentResponse {
	return PaymentResponse{
		ID: p.ID, InvoiceID: p.InvoiceID, InvoiceNumber: p.InvoiceNumber, AccountName: p.AccountName, Amount: p.Amount,
		Method: p.Method, Reference: p.Reference, Note: p.Note, RecordedBy: strPtr(p.RecordedBy), PaidAt: p.PaidAt, CreatedAt: p.CreatedAt,
	}
}

func SettingsFromDomain(s domain.Settings) SettingsResponse {
	channels := s.DefaultChannels
	if channels == nil {
		channels = []string{}
	}
	return SettingsResponse{
		InvoiceLeadDays: s.InvoiceLeadDays, PaymentTermsDays: s.PaymentTermsDays, DefaultChannels: channels,
		DefaultSenderUserID: strPtr(s.DefaultSenderUserID),
	}
}
