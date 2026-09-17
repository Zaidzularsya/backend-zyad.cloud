package service

import (
	"context"
	"testing"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type stubARAPStore struct {
	createTransactionErr  error
	createPaymentErr      error
	lastTransactionParams repository.CreateARAPTransactionParams
	lastPaymentParams     repository.CreateARAPPaymentParams
}

func (s *stubARAPStore) CreatePartner(_ context.Context, params repository.CreateBusinessPartnerParams) (model.BusinessPartner, error) {
	return model.BusinessPartner{ID: "partner-1", PartnerType: params.PartnerType, Code: params.Code, Name: params.Name}, nil
}

func (s *stubARAPStore) UpdatePartner(_ context.Context, params repository.UpdateBusinessPartnerParams) (model.BusinessPartner, error) {
	return model.BusinessPartner{ID: params.ID, Name: params.Name}, nil
}

func (s *stubARAPStore) FindPartnerByID(_ context.Context, id string) (model.BusinessPartner, error) {
	return model.BusinessPartner{ID: id}, nil
}

func (s *stubARAPStore) ListPartners(_ context.Context, _ repository.BusinessPartnerListFilter) ([]model.BusinessPartner, error) {
	return nil, nil
}

func (s *stubARAPStore) CreateTransaction(_ context.Context, params repository.CreateARAPTransactionParams) (model.ARAPTransaction, error) {
	s.lastTransactionParams = params
	if s.createTransactionErr != nil {
		return model.ARAPTransaction{}, s.createTransactionErr
	}
	return model.ARAPTransaction{ID: "tx-1", PartnerID: params.PartnerID, TransactionType: params.TransactionType, Amount: params.Amount}, nil
}

func (s *stubARAPStore) FindTransactionByID(_ context.Context, id string) (model.ARAPTransaction, error) {
	return model.ARAPTransaction{ID: id}, nil
}

func (s *stubARAPStore) ListTransactions(_ context.Context, _ repository.ARAPTransactionListFilter) ([]model.ARAPTransaction, int64, error) {
	return nil, 0, nil
}

func (s *stubARAPStore) CreatePayment(_ context.Context, params repository.CreateARAPPaymentParams) (model.ARAPPayment, error) {
	s.lastPaymentParams = params
	if s.createPaymentErr != nil {
		return model.ARAPPayment{}, s.createPaymentErr
	}
	return model.ARAPPayment{ID: "pmt-1", Amount: params.Amount}, nil
}

func (s *stubARAPStore) AgingReport(_ context.Context, _ model.ARAPTransactionType, _ time.Time) ([]repository.AgingBucketRow, error) {
	return nil, nil
}

type stubEntryNumberGenerator struct{}

func (stubEntryNumberGenerator) NextEntryNumber(_ context.Context, _ int) (string, error) {
	return "JE-2025-000001", nil
}

func appErrorCode(t *testing.T, err error) string {
	t.Helper()
	appErr, ok := err.(*coreerrors.AppError)
	if !ok {
		t.Fatalf("expected *coreerrors.AppError, got %T (%v)", err, err)
	}
	return appErr.Code
}

func TestCreateTransactionRejectsInvalidTransactionType(t *testing.T) {
	svc := NewARAPService(&stubARAPStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateARAPTransactionRequest{
		PartnerID: "partner-1", TransactionType: "bogus", TransactionDate: "2025-06-15", DueDate: "2025-07-15",
		Amount: "100.00", ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for invalid transaction_type, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateTransactionRejectsZeroAmount(t *testing.T) {
	svc := NewARAPService(&stubARAPStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateARAPTransactionRequest{
		PartnerID: "partner-1", TransactionType: "receivable", TransactionDate: "2025-06-15", DueDate: "2025-07-15",
		Amount: "0", ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for zero amount, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateTransactionMapsPartnerTypeMismatch(t *testing.T) {
	store := &stubARAPStore{createTransactionErr: repository.ErrARAPTransactionTypeMismatch}
	svc := NewARAPService(store, stubEntryNumberGenerator{})

	_, err := svc.CreateTransaction(context.Background(), dto.CreateARAPTransactionRequest{
		PartnerID: "partner-1", TransactionType: "receivable", TransactionDate: "2025-06-15", DueDate: "2025-07-15",
		Amount: "100.00", ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for partner/transaction type mismatch, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreatePaymentRejectsOverpayment(t *testing.T) {
	store := &stubARAPStore{createPaymentErr: repository.ErrARAPOverpayment}
	svc := NewARAPService(store, stubEntryNumberGenerator{})

	_, err := svc.CreatePayment(context.Background(), dto.CreateARAPPaymentRequest{
		PartnerID: "partner-1", ARAPTransactionID: "tx-1", PaymentDate: "2025-06-20",
		Amount: "999999.00", CashBankAccountID: "cba-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for overpayment, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreatePaymentRejectsZeroAmount(t *testing.T) {
	svc := NewARAPService(&stubARAPStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreatePayment(context.Background(), dto.CreateARAPPaymentRequest{
		PartnerID: "partner-1", ARAPTransactionID: "tx-1", PaymentDate: "2025-06-20",
		Amount: "-10.00", CashBankAccountID: "cba-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for negative amount, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}
