package service

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type stubTaxStore struct {
	createTransactionErr error
	lastParams           repository.CreateTaxTransactionParams
}

func (s *stubTaxStore) ListTypes(_ context.Context) ([]model.TaxType, error) {
	return nil, nil
}

func (s *stubTaxStore) CreateRate(_ context.Context, params repository.CreateTaxRateParams) (model.TaxRate, error) {
	return model.TaxRate{ID: "rate-1", TaxTypeID: params.TaxTypeID, RatePercent: params.RatePercent}, nil
}

func (s *stubTaxStore) ListRates(_ context.Context, _ string) ([]model.TaxRate, error) {
	return nil, nil
}

func (s *stubTaxStore) CreateTransaction(_ context.Context, params repository.CreateTaxTransactionParams) (model.TaxTransaction, error) {
	s.lastParams = params
	if s.createTransactionErr != nil {
		return model.TaxTransaction{}, s.createTransactionErr
	}
	return model.TaxTransaction{ID: "tax-tx-1", TaxTypeID: params.TaxTypeID, Amount: params.Amount, Direction: params.Direction}, nil
}

func (s *stubTaxStore) FindTransactionByID(_ context.Context, id string) (model.TaxTransaction, error) {
	return model.TaxTransaction{ID: id}, nil
}

func (s *stubTaxStore) ListTransactions(_ context.Context, _ repository.TaxTransactionListFilter) ([]model.TaxTransaction, int64, error) {
	return nil, 0, nil
}

func (s *stubTaxStore) Summary(_ context.Context, _, _ time.Time) ([]model.TaxSummaryRow, error) {
	return nil, nil
}

func TestCreateTaxTransactionRejectsInvalidDirection(t *testing.T) {
	svc := NewTaxService(&stubTaxStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateTaxTransactionRequest{
		TaxTypeID: "type-1", TransactionDate: "2025-03-31", Amount: "1100000",
		Direction: "sideways", TaxAccountID: "acc-1", ContraAccountID: "acc-2",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for invalid direction, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateTaxTransactionRejectsZeroAmount(t *testing.T) {
	svc := NewTaxService(&stubTaxStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateTaxTransactionRequest{
		TaxTypeID: "type-1", TransactionDate: "2025-03-31", Amount: "0",
		Direction: "increase", TaxAccountID: "acc-1", ContraAccountID: "acc-2",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for zero amount, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateTaxTransactionPassesDirectionThrough(t *testing.T) {
	store := &stubTaxStore{}
	svc := NewTaxService(store, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateTaxTransactionRequest{
		TaxTypeID: "type-1", TransactionDate: "2025-03-31", Amount: "1100000",
		Direction: "decrease", TaxAccountID: "acc-1", ContraAccountID: "acc-2",
	}, "user-1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store.lastParams.Direction != model.TaxDirectionDecrease {
		t.Fatalf("expected direction 'decrease' to reach the store, got %q", store.lastParams.Direction)
	}
}

func TestCreateTaxRateRejectsNegativeRate(t *testing.T) {
	svc := NewTaxService(&stubTaxStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateRate(context.Background(), dto.CreateTaxRateRequest{
		TaxTypeID: "type-1", RatePercent: "-1", EffectiveDate: "2025-01-01",
	})

	if err == nil {
		t.Fatal("expected an error for negative rate_percent, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}
