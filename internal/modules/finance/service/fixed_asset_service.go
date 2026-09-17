package service

import (
	"context"
	"math/big"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance"
	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type FixedAssetStore interface {
	CreateCategory(ctx context.Context, params repository.CreateAssetCategoryParams) (model.AssetCategory, error)
	FindCategoryByID(ctx context.Context, id string) (model.AssetCategory, error)
	ListCategories(ctx context.Context, filter repository.AssetCategoryListFilter) ([]model.AssetCategory, error)
	CreateAsset(ctx context.Context, params repository.CreateFixedAssetParams, schedule []repository.ScheduleAmount) (model.FixedAsset, error)
	FindAssetByID(ctx context.Context, id string) (model.FixedAsset, error)
	ListAssets(ctx context.Context, filter repository.FixedAssetListFilter) ([]model.FixedAsset, int64, error)
	ListSchedules(ctx context.Context, fixedAssetID string) ([]model.DepreciationSchedule, error)
	PostDepreciationThroughDate(ctx context.Context, asOf time.Time, entryNumbers func(year int) (string, error), actorID *string) ([]repository.DepreciationPostResult, error)
}

type FixedAssetService struct {
	store       FixedAssetStore
	entryNumber EntryNumberGenerator
}

func NewFixedAssetService(store FixedAssetStore, entryNumber EntryNumberGenerator) *FixedAssetService {
	return &FixedAssetService{store: store, entryNumber: entryNumber}
}

func (s *FixedAssetService) CreateCategory(ctx context.Context, request dto.CreateAssetCategoryRequest) (dto.AssetCategoryResponse, error) {
	if strings.TrimSpace(request.Code) == "" || strings.TrimSpace(request.Name) == "" {
		return dto.AssetCategoryResponse{}, finance.ValidationError("code and name are required")
	}
	if request.DefaultUsefulLifeMonths != nil && *request.DefaultUsefulLifeMonths <= 0 {
		return dto.AssetCategoryResponse{}, finance.ValidationError("default_useful_life_months must be greater than zero")
	}
	category, err := s.store.CreateCategory(ctx, repository.CreateAssetCategoryParams{
		Code:                             request.Code,
		Name:                             request.Name,
		AssetAccountID:                   strings.TrimSpace(request.AssetAccountID),
		AccumulatedDepreciationAccountID: strings.TrimSpace(request.AccumulatedDepreciationAccountID),
		DepreciationExpenseAccountID:     strings.TrimSpace(request.DepreciationExpenseAccountID),
		DefaultUsefulLifeMonths:          request.DefaultUsefulLifeMonths,
	})
	if err != nil {
		return dto.AssetCategoryResponse{}, mapAssetCategoryError(err)
	}
	return assetCategoryResponse(category), nil
}

func (s *FixedAssetService) ListCategories(ctx context.Context, query dto.AssetCategoryListQuery) ([]dto.AssetCategoryResponse, error) {
	categories, err := s.store.ListCategories(ctx, repository.AssetCategoryListFilter{IncludeInactive: query.IncludeInactive})
	if err != nil {
		return nil, err
	}
	responses := make([]dto.AssetCategoryResponse, 0, len(categories))
	for _, c := range categories {
		responses = append(responses, assetCategoryResponse(c))
	}
	return responses, nil
}

func (s *FixedAssetService) CreateAsset(ctx context.Context, request dto.CreateFixedAssetRequest, actorID string) (dto.FixedAssetResponse, error) {
	if strings.TrimSpace(request.AssetCode) == "" || strings.TrimSpace(request.AssetName) == "" {
		return dto.FixedAssetResponse{}, finance.ValidationError("asset_code and asset_name are required")
	}
	if request.UsefulLifeMonths <= 0 {
		return dto.FixedAssetResponse{}, finance.ValidationError("useful_life_months must be greater than zero")
	}
	acquisitionDate, err := parseRequiredDate(request.AcquisitionDate)
	if err != nil {
		return dto.FixedAssetResponse{}, err
	}
	cost, err := moneyRat(request.AcquisitionCost)
	if err != nil || cost.Sign() <= 0 {
		return dto.FixedAssetResponse{}, finance.ValidationError("acquisition_cost must be greater than zero")
	}
	salvage := zeroRat()
	if strings.TrimSpace(request.SalvageValue) != "" {
		salvage, err = moneyRat(request.SalvageValue)
		if err != nil || salvage.Sign() < 0 {
			return dto.FixedAssetResponse{}, finance.ValidationError("salvage_value must not be negative")
		}
	}
	if salvage.Cmp(cost) >= 0 {
		return dto.FixedAssetResponse{}, finance.ValidationError("salvage_value must be less than acquisition_cost")
	}

	entryNumber, err := s.entryNumber.NextEntryNumber(ctx, acquisitionDate.Year())
	if err != nil {
		return dto.FixedAssetResponse{}, err
	}

	schedule := generateStraightLineSchedule(acquisitionDate, cost, salvage, request.UsefulLifeMonths)

	asset, err := s.store.CreateAsset(ctx, repository.CreateFixedAssetParams{
		AssetCategoryID:  strings.TrimSpace(request.AssetCategoryID),
		AssetCode:        request.AssetCode,
		AssetName:        request.AssetName,
		AcquisitionDate:  acquisitionDate,
		AcquisitionCost:  formatMoney(cost),
		SalvageValue:     formatMoney(salvage),
		UsefulLifeMonths: request.UsefulLifeMonths,
		Description:      request.Description,
		ContraAccountID:  strings.TrimSpace(request.ContraAccountID),
		EntryNumber:      entryNumber,
		CreatedBy:        trimmedOrNil(actorID),
	}, schedule)
	if err != nil {
		return dto.FixedAssetResponse{}, mapFixedAssetError(err)
	}
	return fixedAssetResponse(asset), nil
}

func (s *FixedAssetService) ListAssets(ctx context.Context, query dto.FixedAssetListQuery) (dto.FixedAssetListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	assets, total, err := s.store.ListAssets(ctx, repository.FixedAssetListFilter{
		AssetCategoryID: query.AssetCategoryID,
		Status:          model.AssetStatus(strings.TrimSpace(query.Status)),
		Limit:           perPage,
		Offset:          (page - 1) * perPage,
	})
	if err != nil {
		return dto.FixedAssetListResponse{}, err
	}
	responses := make([]dto.FixedAssetResponse, 0, len(assets))
	for _, a := range assets {
		responses = append(responses, fixedAssetResponse(a))
	}
	return dto.FixedAssetListResponse{
		Items: responses,
		Meta:  dto.PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPagesOf(perPage, total)},
	}, nil
}

func (s *FixedAssetService) ListSchedules(ctx context.Context, fixedAssetID string) ([]dto.DepreciationScheduleResponse, error) {
	schedules, err := s.store.ListSchedules(ctx, fixedAssetID)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.DepreciationScheduleResponse, 0, len(schedules))
	for _, sched := range schedules {
		responses = append(responses, depreciationScheduleResponse(sched))
	}
	return responses, nil
}

func (s *FixedAssetService) PostDepreciation(ctx context.Context, request dto.PostDepreciationRequest, actorID string) (dto.PostDepreciationResponse, error) {
	asOf, err := parseRequiredDate(request.AsOfDate)
	if err != nil {
		return dto.PostDepreciationResponse{}, err
	}
	entryNumbers := func(year int) (string, error) { return s.entryNumber.NextEntryNumber(ctx, year) }
	results, err := s.store.PostDepreciationThroughDate(ctx, asOf, entryNumbers, trimmedOrNil(actorID))
	if err != nil {
		return dto.PostDepreciationResponse{}, err
	}

	response := dto.PostDepreciationResponse{Items: make([]dto.PostDepreciationResultItem, 0, len(results))}
	for _, r := range results {
		if r.Status == "posted" {
			response.PostedCount++
		} else {
			response.FailedCount++
		}
		response.Items = append(response.Items, dto.PostDepreciationResultItem{
			ScheduleID: r.ScheduleID,
			AssetCode:  r.AssetCode,
			PeriodDate: formatDate(r.PeriodDate),
			Status:     r.Status,
			Reason:     r.Reason,
		})
	}
	return response, nil
}

// generateStraightLineSchedule computes one depreciation amount per month
// for usefulLifeMonths, starting in the acquisition month (counted in full).
// Amounts are derived from the *cumulative* depreciation at each period,
// rounded to 2dp, so the schedule always sums to exactly cost - salvage —
// any rounding remainder lands in the last period instead of drifting.
func generateStraightLineSchedule(acquisitionDate time.Time, cost, salvage *big.Rat, usefulLifeMonths int) []repository.ScheduleAmount {
	depreciableBase := new(big.Rat).Sub(cost, salvage)
	schedule := make([]repository.ScheduleAmount, usefulLifeMonths)
	previousCumulative := zeroRat()
	for i := 1; i <= usefulLifeMonths; i++ {
		fraction := big.NewRat(int64(i), int64(usefulLifeMonths))
		cumulative := roundMoney(new(big.Rat).Mul(depreciableBase, fraction))
		periodAmount := new(big.Rat).Sub(cumulative, previousCumulative)
		schedule[i-1] = repository.ScheduleAmount{
			SequenceNumber: i,
			PeriodDate:     lastDayOfMonth(acquisitionDate.AddDate(0, i-1, 0)),
			Amount:         formatMoney(periodAmount),
		}
		previousCumulative = cumulative
	}
	return schedule
}

func lastDayOfMonth(t time.Time) time.Time {
	firstOfNextMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	return firstOfNextMonth.AddDate(0, 0, -1)
}

func assetCategoryResponse(c model.AssetCategory) dto.AssetCategoryResponse {
	return dto.AssetCategoryResponse{
		ID:                                 c.ID,
		Code:                               c.Code,
		Name:                               c.Name,
		AssetAccountID:                     c.AssetAccountID,
		AssetAccountCode:                   c.AssetAccountCode,
		AssetAccountName:                   c.AssetAccountName,
		AccumulatedDepreciationAccountID:   c.AccumulatedDepreciationAccountID,
		AccumulatedDepreciationAccountCode: c.AccumulatedDepreciationAccountCode,
		AccumulatedDepreciationAccountName: c.AccumulatedDepreciationAccountName,
		DepreciationExpenseAccountID:       c.DepreciationExpenseAccountID,
		DepreciationExpenseAccountCode:     c.DepreciationExpenseAccountCode,
		DepreciationExpenseAccountName:     c.DepreciationExpenseAccountName,
		DefaultUsefulLifeMonths:            c.DefaultUsefulLifeMonths,
		IsActive:                           c.IsActive,
		CreatedAt:                          formatTime(c.CreatedAt),
		UpdatedAt:                          formatTime(c.UpdatedAt),
	}
}

func fixedAssetResponse(a model.FixedAsset) dto.FixedAssetResponse {
	return dto.FixedAssetResponse{
		ID:                        a.ID,
		AssetCategoryID:           a.AssetCategoryID,
		AssetCategoryCode:         a.AssetCategoryCode,
		AssetCategoryName:         a.AssetCategoryName,
		AssetCode:                 a.AssetCode,
		AssetName:                 a.AssetName,
		AcquisitionDate:           formatDate(a.AcquisitionDate),
		AcquisitionCost:           a.AcquisitionCost,
		SalvageValue:              a.SalvageValue,
		UsefulLifeMonths:          a.UsefulLifeMonths,
		Description:               a.Description,
		Status:                    string(a.Status),
		AcquisitionJournalEntryID: a.AcquisitionJournalEntryID,
		AccumulatedDepreciation:   a.AccumulatedDepreciation,
		BookValue:                 a.BookValue,
		CreatedAt:                 formatTime(a.CreatedAt),
		UpdatedAt:                 formatTime(a.UpdatedAt),
	}
}

func depreciationScheduleResponse(s model.DepreciationSchedule) dto.DepreciationScheduleResponse {
	return dto.DepreciationScheduleResponse{
		ID:                 s.ID,
		SequenceNumber:     s.SequenceNumber,
		PeriodDate:         formatDate(s.PeriodDate),
		DepreciationAmount: s.DepreciationAmount,
		Status:             string(s.Status),
		JournalEntryID:     s.JournalEntryID,
		PostedAt:           formatOptionalTime(s.PostedAt),
	}
}
