package service

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type stubInvoiceStore struct {
	createdParams repository.CreateInvoiceParams
	items         []model.InvoiceItem
}

func (s *stubInvoiceStore) Create(
	_ context.Context,
	params repository.CreateInvoiceParams,
) (model.Invoice, error) {
	s.createdParams = params
	return model.Invoice{
		ID:             "invoice-1",
		OrganizationID: params.OrganizationID,
		SubscriptionID: params.SubscriptionID,
		InvoiceNumber:  params.InvoiceNumber,
		Status:         params.Status,
		Currency:       params.Currency,
		SubtotalAmount: params.SubtotalAmount,
		DiscountAmount: params.DiscountAmount,
		TaxAmount:      params.TaxAmount,
		TotalAmount:    params.TotalAmount,
		DueDate:        params.DueDate,
		Metadata:       params.Metadata,
	}, nil
}

func (s *stubInvoiceStore) FindByID(
	context.Context,
	string,
	string,
) (model.Invoice, error) {
	return model.Invoice{}, nil
}

func (s *stubInvoiceStore) List(
	context.Context,
	repository.InvoiceListFilter,
) ([]model.Invoice, int64, error) {
	return nil, 0, nil
}

func (s *stubInvoiceStore) ListAllOrganizations(
	context.Context,
	repository.InvoiceListFilter,
) ([]model.Invoice, int64, error) {
	return nil, 0, nil
}

func (s *stubInvoiceStore) ListItems(context.Context, string) ([]model.InvoiceItem, error) {
	return s.items, nil
}

func (s *stubInvoiceStore) UpdateStatus(
	context.Context,
	repository.UpdateInvoiceStatusParams,
) (model.Invoice, error) {
	return model.Invoice{}, nil
}

func TestInvoiceServiceCreateCalculatesTotals(t *testing.T) {
	store := &stubInvoiceStore{}
	service := NewInvoiceService(store)

	response, err := service.Create(context.Background(), dto.CreateInvoiceRequest{
		OrganizationID: "organization-1",
		Currency:       "idr",
		Items: []dto.CreateInvoiceItemRequest{
			{
				Type:        "subscription",
				Description: "Growth monthly subscription",
				Quantity:    "2",
				UnitAmount:  "100000",
			},
			{
				Type:        "tax",
				Description: "VAT",
				Quantity:    "1",
				UnitAmount:  "22000",
			},
			{
				Type:        "discount",
				Description: "Promo",
				Quantity:    "1",
				UnitAmount:  "5000",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if response.SubtotalAmount != "200000.00" {
		t.Fatalf("SubtotalAmount = %s, want 200000.00", response.SubtotalAmount)
	}
	if response.TaxAmount != "22000.00" {
		t.Fatalf("TaxAmount = %s, want 22000.00", response.TaxAmount)
	}
	if response.DiscountAmount != "5000.00" {
		t.Fatalf("DiscountAmount = %s, want 5000.00", response.DiscountAmount)
	}
	if response.TotalAmount != "217000.00" {
		t.Fatalf("TotalAmount = %s, want 217000.00", response.TotalAmount)
	}
	if len(store.createdParams.Items) != 3 {
		t.Fatalf("created items = %d, want 3", len(store.createdParams.Items))
	}
	if store.createdParams.Items[0].TotalAmount != "200000.00" {
		t.Fatalf("first item total = %s, want 200000.00", store.createdParams.Items[0].TotalAmount)
	}
	if store.createdParams.Currency != "IDR" {
		t.Fatalf("Currency = %s, want IDR", store.createdParams.Currency)
	}
}

func TestInvoiceServiceCreateRejectsNegativeTotal(t *testing.T) {
	service := NewInvoiceService(&stubInvoiceStore{})

	_, err := service.Create(context.Background(), dto.CreateInvoiceRequest{
		OrganizationID: "organization-1",
		Items: []dto.CreateInvoiceItemRequest{
			{
				Type:        "discount",
				Description: "Too much discount",
				Quantity:    "1",
				UnitAmount:  "1000",
			},
		},
	})
	if err == nil {
		t.Fatal("Create error = nil, want validation error")
	}
}
