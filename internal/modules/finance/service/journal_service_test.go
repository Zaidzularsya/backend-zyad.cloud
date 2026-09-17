package service

import (
	"context"
	"testing"
	"time"

	finance "zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"

	coreerrors "zyad.cloud/internal/core/errors"
)

type stubJournalStore struct {
	created      []repository.CreateJournalEntryParams
	createCalled bool
	returned     model.JournalEntry
}

func (s *stubJournalStore) Create(_ context.Context, params repository.CreateJournalEntryParams) (model.JournalEntry, error) {
	s.createCalled = true
	s.created = append(s.created, params)
	return model.JournalEntry{ID: "entry-1", EntryNumber: params.EntryNumber, EntryDate: params.EntryDate, Lines: nil}, nil
}

func (s *stubJournalStore) FindByID(_ context.Context, id string) (model.JournalEntry, error) {
	return model.JournalEntry{ID: id}, nil
}

func (s *stubJournalStore) List(_ context.Context, _ repository.JournalEntryListFilter) ([]model.JournalEntry, int64, error) {
	return nil, 0, nil
}

func (s *stubJournalStore) NextEntryNumber(_ context.Context, year int) (string, error) {
	return "JE-2025-000001", nil
}

func (s *stubJournalStore) Reverse(_ context.Context, _ string, _ time.Time, _ string, _ string, _ *string) (model.JournalEntry, error) {
	return model.JournalEntry{}, nil
}

func TestCreateJournalEntryRejectsUnbalancedLines(t *testing.T) {
	store := &stubJournalStore{}
	svc := NewJournalService(store)

	_, err := svc.Create(context.Background(), dto.CreateJournalEntryRequest{
		EntryDate: "2025-06-15",
		Lines: []dto.CreateJournalLineRequest{
			{AccountID: "acc-1", Debit: "100.00"},
			{AccountID: "acc-2", Credit: "90.00"},
		},
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for unbalanced journal lines, got nil")
	}
	appErr, ok := err.(*coreerrors.AppError)
	if !ok {
		t.Fatalf("expected *coreerrors.AppError, got %T: %v", err, err)
	}
	if appErr.Code != finance.ErrCodeJournalUnbalanced {
		t.Fatalf("expected error code %s, got %s", finance.ErrCodeJournalUnbalanced, appErr.Code)
	}
	if store.createCalled {
		t.Fatal("expected repository Create to never be called for an unbalanced entry")
	}
}

func TestCreateJournalEntryAcceptsBalancedLines(t *testing.T) {
	store := &stubJournalStore{}
	svc := NewJournalService(store)

	_, err := svc.Create(context.Background(), dto.CreateJournalEntryRequest{
		EntryDate: "2025-06-15",
		Lines: []dto.CreateJournalLineRequest{
			{AccountID: "acc-1", Debit: "150.50"},
			{AccountID: "acc-2", Credit: "150.50"},
		},
	}, "user-1")

	if err != nil {
		t.Fatalf("expected balanced entry to succeed, got error: %v", err)
	}
	if !store.createCalled {
		t.Fatal("expected repository Create to be called for a balanced entry")
	}
	if len(store.created) != 1 {
		t.Fatalf("expected exactly one Create call, got %d", len(store.created))
	}
}

func TestCreateJournalEntryRejectsLineWithBothDebitAndCredit(t *testing.T) {
	store := &stubJournalStore{}
	svc := NewJournalService(store)

	_, err := svc.Create(context.Background(), dto.CreateJournalEntryRequest{
		EntryDate: "2025-06-15",
		Lines: []dto.CreateJournalLineRequest{
			{AccountID: "acc-1", Debit: "50.00", Credit: "50.00"},
			{AccountID: "acc-2", Credit: "50.00"},
		},
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error when a line has both debit and credit set")
	}
	if store.createCalled {
		t.Fatal("expected repository Create to never be called")
	}
}

func TestCreateJournalEntryRequiresAtLeastTwoLines(t *testing.T) {
	store := &stubJournalStore{}
	svc := NewJournalService(store)

	_, err := svc.Create(context.Background(), dto.CreateJournalEntryRequest{
		EntryDate: "2025-06-15",
		Lines: []dto.CreateJournalLineRequest{
			{AccountID: "acc-1", Debit: "50.00"},
		},
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for a single-line journal entry")
	}
	if store.createCalled {
		t.Fatal("expected repository Create to never be called")
	}
}
