package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/service"
)

// OnboardNewContactRequest is a contact the onboarding creates.
type OnboardNewContactRequest struct {
	Name  string `json:"name" binding:"max=200"`
	Phone string `json:"phone" binding:"max=50"`
	Email string `json:"email" binding:"max=255"`
}

// OnboardCustomerRequest: contact_id (existing CRM contact) or new.
type OnboardCustomerRequest struct {
	ContactID string                    `json:"contact_id"`
	New       *OnboardNewContactRequest `json:"new"`
}

// OnboardRequest is the body of POST /secrets/:id/onboard[/preview].
type OnboardRequest struct {
	Customer             OnboardCustomerRequest `json:"customer"`
	ProductID            string                 `json:"product_id" binding:"required"`
	Quantity             string                 `json:"quantity"`
	UnitPrice            string                 `json:"unit_price"`
	BillingDay           int                    `json:"billing_day"`
	CreateInitialInvoice bool                   `json:"create_initial_invoice"`
	Channels             []string               `json:"channels"`
}

func (r OnboardRequest) Input(secretID string) service.OnboardInput {
	in := service.OnboardInput{
		SecretID: secretID, ProductID: r.ProductID, Quantity: r.Quantity, UnitPrice: r.UnitPrice,
		BillingDay: r.BillingDay, CreateInitialInvoice: r.CreateInitialInvoice, Channels: r.Channels,
		Customer: service.CustomerChoice{ExistingContactID: r.Customer.ContactID},
	}
	if r.Customer.New != nil {
		in.Customer.New = &service.NewContact{Name: r.Customer.New.Name, Phone: r.Customer.New.Phone, Email: r.Customer.New.Email}
	}
	return in
}

type OnboardContactResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func onboardContact(c service.OnboardContactRef) OnboardContactResponse {
	return OnboardContactResponse{ID: c.ID, Name: c.Name, Email: c.Email, Phone: c.Phone}
}

type OnboardPlanResponse struct {
	SecretName        string                  `json:"secret_name"`
	ContactAction     string                  `json:"contact_action"`
	Contact           OnboardContactResponse  `json:"contact"`
	ProductName       string                  `json:"product_name"`
	Quantity          string                  `json:"quantity"`
	UnitPrice         string                  `json:"unit_price"`
	DealTitle         string                  `json:"deal_title"`
	ContractStartDate string                  `json:"contract_start_date"`
	InitialInvoice    bool                    `json:"initial_invoice"`
	Channels          []string                `json:"channels"`
	DuplicateOf       *OnboardContactResponse `json:"duplicate_of"`
}

func OnboardPlanFromService(p service.OnboardPlan) OnboardPlanResponse {
	out := OnboardPlanResponse{
		SecretName: p.SecretName, ContactAction: p.ContactAction, Contact: onboardContact(p.Contact), ProductName: p.ProductName,
		Quantity: p.Quantity, UnitPrice: p.UnitPrice, DealTitle: p.DealTitle, ContractStartDate: p.ContractStartDate.Format("2006-01-02"),
		InitialInvoice: p.InitialInvoice, Channels: p.Channels,
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}
	if p.DuplicateOf != nil {
		d := onboardContact(*p.DuplicateOf)
		out.DuplicateOf = &d
	}
	return out
}

// ContractRefResponse identifies a receivable contract.
type ContractRefResponse struct {
	ID     string `json:"id"`
	Number string `json:"number"`
}

// SecretDetailResponse is GET /secrets/:id.
type SecretDetailResponse struct {
	Secret     SecretResponse        `json:"secret"`
	Onboarding *OnboardStateResponse `json:"onboarding"`
	Contract   *ContractRefResponse  `json:"contract"`
	CanOnboard bool                  `json:"can_onboard"`
}

func SecretDetailFromService(d service.SecretDetail) SecretDetailResponse {
	out := SecretDetailResponse{Secret: SecretFromDomain(d.Secret), CanOnboard: d.CanOnboard}
	if d.Onboarding != nil {
		st := OnboardStateFromService(*d.Onboarding)
		out.Onboarding = &st
	}
	if d.Contract != nil {
		out.Contract = &ContractRefResponse{ID: d.Contract.ID, Number: d.Contract.Number}
	}
	return out
}

// OnboardStateResponse is the stored progress; ids are "" until their stage completes.
type OnboardStateResponse struct {
	SecretID     string     `json:"secret_id"`
	Stage        string     `json:"stage"`
	ContactID    string     `json:"contact_id"`
	DealID       string     `json:"deal_id"`
	QuotationID  string     `json:"quotation_id"`
	SalesOrderID string     `json:"sales_order_id"`
	ContractID   string     `json:"contract_id"`
	AccountID    string     `json:"account_id"`
	LastError    string     `json:"last_error"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func OnboardStateFromService(s service.OnboardState) OnboardStateResponse {
	out := OnboardStateResponse{
		SecretID: s.SecretID, Stage: s.Stage, ContactID: s.ContactID, DealID: s.DealID, QuotationID: s.QuotationID,
		SalesOrderID: s.SalesOrderID, ContractID: s.ContractID, AccountID: s.AccountID, LastError: s.LastError,
	}
	if !s.UpdatedAt.IsZero() {
		t := s.UpdatedAt
		out.UpdatedAt = &t
	}
	return out
}
