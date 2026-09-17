package service

import (
	"context"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
)

type FiscalStore interface {
	CreateFiscalYear(ctx context.Context, year int) (model.FiscalYear, error)
	ListFiscalYears(ctx context.Context) ([]model.FiscalYear, error)
	ListFiscalPeriods(ctx context.Context, fiscalYearID string) ([]model.FiscalPeriod, error)
	FindPeriodByID(ctx context.Context, id string) (model.FiscalPeriod, error)
	SetPeriodStatus(ctx context.Context, id string, status model.FiscalStatus, actorID *string) (model.FiscalPeriod, error)
	SetFiscalYearStatus(ctx context.Context, id string, status model.FiscalStatus, actorID *string) (model.FiscalYear, error)
}

type FiscalService struct {
	store FiscalStore
}

func NewFiscalService(store FiscalStore) *FiscalService {
	return &FiscalService{store: store}
}

func (s *FiscalService) CreateFiscalYear(ctx context.Context, request dto.CreateFiscalYearRequest) (dto.FiscalYearResponse, error) {
	if request.Year < 2000 || request.Year > 2100 {
		return dto.FiscalYearResponse{}, finance.ValidationError("year is out of range")
	}
	year, err := s.store.CreateFiscalYear(ctx, request.Year)
	if err != nil {
		return dto.FiscalYearResponse{}, mapFiscalYearError(err)
	}
	periods, err := s.store.ListFiscalPeriods(ctx, year.ID)
	if err != nil {
		return dto.FiscalYearResponse{}, err
	}
	return fiscalYearResponse(year, periods), nil
}

func (s *FiscalService) ListFiscalYears(ctx context.Context) ([]dto.FiscalYearResponse, error) {
	years, err := s.store.ListFiscalYears(ctx)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.FiscalYearResponse, 0, len(years))
	for _, y := range years {
		periods, err := s.store.ListFiscalPeriods(ctx, y.ID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, fiscalYearResponse(y, periods))
	}
	return responses, nil
}

func (s *FiscalService) ClosePeriod(ctx context.Context, periodID string, actorID string) (dto.FiscalPeriodResponse, error) {
	period, err := s.store.SetPeriodStatus(ctx, periodID, model.FiscalStatusClosed, trimmedOrNil(actorID))
	if err != nil {
		return dto.FiscalPeriodResponse{}, mapFiscalPeriodError(err)
	}
	return fiscalPeriodResponse(period), nil
}

func (s *FiscalService) ReopenPeriod(ctx context.Context, periodID string, actorID string) (dto.FiscalPeriodResponse, error) {
	period, err := s.store.SetPeriodStatus(ctx, periodID, model.FiscalStatusOpen, trimmedOrNil(actorID))
	if err != nil {
		return dto.FiscalPeriodResponse{}, mapFiscalPeriodError(err)
	}
	return fiscalPeriodResponse(period), nil
}

func (s *FiscalService) CloseFiscalYear(ctx context.Context, fiscalYearID string, actorID string) (dto.FiscalYearResponse, error) {
	year, err := s.store.SetFiscalYearStatus(ctx, fiscalYearID, model.FiscalStatusClosed, trimmedOrNil(actorID))
	if err != nil {
		return dto.FiscalYearResponse{}, mapFiscalYearError(err)
	}
	periods, err := s.store.ListFiscalPeriods(ctx, year.ID)
	if err != nil {
		return dto.FiscalYearResponse{}, err
	}
	return fiscalYearResponse(year, periods), nil
}

func (s *FiscalService) ReopenFiscalYear(ctx context.Context, fiscalYearID string, actorID string) (dto.FiscalYearResponse, error) {
	year, err := s.store.SetFiscalYearStatus(ctx, fiscalYearID, model.FiscalStatusOpen, trimmedOrNil(actorID))
	if err != nil {
		return dto.FiscalYearResponse{}, mapFiscalYearError(err)
	}
	periods, err := s.store.ListFiscalPeriods(ctx, year.ID)
	if err != nil {
		return dto.FiscalYearResponse{}, err
	}
	return fiscalYearResponse(year, periods), nil
}
