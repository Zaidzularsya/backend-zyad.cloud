package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type JournalStore interface {
	Create(ctx context.Context, params repository.CreateJournalEntryParams) (model.JournalEntry, error)
	FindByID(ctx context.Context, id string) (model.JournalEntry, error)
	List(ctx context.Context, filter repository.JournalEntryListFilter) ([]model.JournalEntry, int64, error)
	NextEntryNumber(ctx context.Context, year int) (string, error)
	Reverse(ctx context.Context, originalID string, reversalDate time.Time, entryNumber string, reason string, actorID *string) (model.JournalEntry, error)
}

type JournalService struct {
	store JournalStore
}

func NewJournalService(store JournalStore) *JournalService {
	return &JournalService{store: store}
}

func (s *JournalService) Create(ctx context.Context, request dto.CreateJournalEntryRequest, actorID string) (dto.JournalEntryResponse, error) {
	entryDate, err := parseRequiredDate(request.EntryDate)
	if err != nil {
		return dto.JournalEntryResponse{}, err
	}
	if len(request.Lines) < 2 {
		return dto.JournalEntryResponse{}, finance.ValidationError("a journal entry needs at least two lines")
	}

	lines := make([]repository.CreateJournalLineParams, 0, len(request.Lines))
	totalDebit, totalCredit := zeroRat(), zeroRat()
	for i, line := range request.Lines {
		if strings.TrimSpace(line.AccountID) == "" {
			return dto.JournalEntryResponse{}, finance.ValidationError("line account_id is required")
		}
		debit, err := moneyRat(line.Debit)
		if err != nil {
			return dto.JournalEntryResponse{}, finance.ValidationError("line debit amount is invalid")
		}
		credit, err := moneyRat(line.Credit)
		if err != nil {
			return dto.JournalEntryResponse{}, finance.ValidationError("line credit amount is invalid")
		}
		if debit.Sign() < 0 || credit.Sign() < 0 {
			return dto.JournalEntryResponse{}, finance.ValidationError("line amounts cannot be negative")
		}
		if debit.Sign() > 0 && credit.Sign() > 0 {
			return dto.JournalEntryResponse{}, finance.ValidationError("a line cannot have both debit and credit amounts")
		}
		if debit.Sign() == 0 && credit.Sign() == 0 {
			return dto.JournalEntryResponse{}, finance.ValidationError("each line must have a non-zero debit or credit amount")
		}
		totalDebit.Add(totalDebit, debit)
		totalCredit.Add(totalCredit, credit)
		lines = append(lines, repository.CreateJournalLineParams{
			AccountID:   strings.TrimSpace(line.AccountID),
			Debit:       formatMoney(debit),
			Credit:      formatMoney(credit),
			Description: line.Description,
		})
		_ = i
	}

	// The single most important invariant in this module: reject anything
	// that is not balanced before it ever reaches the database.
	if totalDebit.Cmp(totalCredit) != 0 {
		return dto.JournalEntryResponse{}, finance.JournalUnbalancedError()
	}

	entryNumber, err := s.store.NextEntryNumber(ctx, entryDate.Year())
	if err != nil {
		return dto.JournalEntryResponse{}, err
	}

	entry, err := s.store.Create(ctx, repository.CreateJournalEntryParams{
		EntryNumber: entryNumber,
		EntryDate:   entryDate,
		SourceType:  model.JournalSourceManual,
		Reference:   request.Reference,
		Description: request.Description,
		CreatedBy:   trimmedOrNil(actorID),
		Lines:       lines,
	})
	if err != nil {
		return dto.JournalEntryResponse{}, mapJournalError(err)
	}
	return journalEntryResponse(entry), nil
}

func (s *JournalService) Get(ctx context.Context, id string) (dto.JournalEntryResponse, error) {
	entry, err := s.store.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.JournalEntryResponse{}, mapJournalError(err)
	}
	return journalEntryResponse(entry), nil
}

func (s *JournalService) List(ctx context.Context, query dto.JournalEntryListQuery) (dto.JournalEntryListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	filter := repository.JournalEntryListFilter{
		Status: model.JournalStatus(strings.TrimSpace(query.Status)),
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	}
	if strings.TrimSpace(query.StartDate) != "" {
		start, err := parseRequiredDate(query.StartDate)
		if err != nil {
			return dto.JournalEntryListResponse{}, err
		}
		filter.StartDate = &start
	}
	if strings.TrimSpace(query.EndDate) != "" {
		end, err := parseRequiredDate(query.EndDate)
		if err != nil {
			return dto.JournalEntryListResponse{}, err
		}
		filter.EndDate = &end
	}
	entries, total, err := s.store.List(ctx, filter)
	if err != nil {
		return dto.JournalEntryListResponse{}, err
	}
	return dto.JournalEntryListResponse{
		Items: journalEntryResponses(entries),
		Meta: dto.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPagesOf(perPage, total),
		},
	}, nil
}

func (s *JournalService) Reverse(ctx context.Context, id string, request dto.ReverseJournalEntryRequest, actorID string) (dto.JournalEntryResponse, error) {
	reversalDate := time.Now().UTC()
	if strings.TrimSpace(request.ReversalDate) != "" {
		parsed, err := parseRequiredDate(request.ReversalDate)
		if err != nil {
			return dto.JournalEntryResponse{}, err
		}
		reversalDate = parsed
	}
	entryNumber, err := s.store.NextEntryNumber(ctx, reversalDate.Year())
	if err != nil {
		return dto.JournalEntryResponse{}, err
	}
	entry, err := s.store.Reverse(ctx, strings.TrimSpace(id), reversalDate, entryNumber, request.Reason, trimmedOrNil(actorID))
	if err != nil {
		return dto.JournalEntryResponse{}, mapJournalError(err)
	}
	return journalEntryResponse(entry), nil
}

func mapJournalError(err error) error {
	switch {
	case errors.Is(err, repository.ErrFiscalPeriodClosed):
		return finance.PeriodClosedError()
	case errors.Is(err, repository.ErrNoFiscalPeriod):
		return finance.NoFiscalPeriodError()
	case errors.Is(err, repository.ErrJournalEntryNotPosted):
		return finance.JournalNotPostedError()
	case errors.Is(err, pgx.ErrNoRows):
		return finance.JournalNotFoundError()
	default:
		return err
	}
}
