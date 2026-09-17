package service

import (
	"context"
	"math/big"
	"testing"
	"time"

	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/modules/finance/repository"
)

type stubFixedAssetStore struct {
	createAssetErr error
}

func (s *stubFixedAssetStore) CreateCategory(_ context.Context, params repository.CreateAssetCategoryParams) (model.AssetCategory, error) {
	return model.AssetCategory{ID: "cat-1", Code: params.Code, Name: params.Name}, nil
}

func (s *stubFixedAssetStore) FindCategoryByID(_ context.Context, id string) (model.AssetCategory, error) {
	return model.AssetCategory{ID: id}, nil
}

func (s *stubFixedAssetStore) ListCategories(_ context.Context, _ repository.AssetCategoryListFilter) ([]model.AssetCategory, error) {
	return nil, nil
}

func (s *stubFixedAssetStore) CreateAsset(_ context.Context, params repository.CreateFixedAssetParams, _ []repository.ScheduleAmount) (model.FixedAsset, error) {
	if s.createAssetErr != nil {
		return model.FixedAsset{}, s.createAssetErr
	}
	return model.FixedAsset{ID: "asset-1", AssetCode: params.AssetCode, AcquisitionCost: params.AcquisitionCost}, nil
}

func (s *stubFixedAssetStore) FindAssetByID(_ context.Context, id string) (model.FixedAsset, error) {
	return model.FixedAsset{ID: id}, nil
}

func (s *stubFixedAssetStore) ListAssets(_ context.Context, _ repository.FixedAssetListFilter) ([]model.FixedAsset, int64, error) {
	return nil, 0, nil
}

func (s *stubFixedAssetStore) ListSchedules(_ context.Context, _ string) ([]model.DepreciationSchedule, error) {
	return nil, nil
}

func (s *stubFixedAssetStore) PostDepreciationThroughDate(_ context.Context, _ time.Time, _ func(year int) (string, error), _ *string) ([]repository.DepreciationPostResult, error) {
	return nil, nil
}

func TestCreateAssetRejectsZeroCost(t *testing.T) {
	svc := NewFixedAssetService(&stubFixedAssetStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateAsset(context.Background(), dto.CreateFixedAssetRequest{
		AssetCategoryID: "cat-1", AssetCode: "AST-001", AssetName: "Laptop", AcquisitionDate: "2025-01-15",
		AcquisitionCost: "0", UsefulLifeMonths: 36, ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for zero acquisition_cost, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateAssetRejectsSalvageAtOrAboveCost(t *testing.T) {
	svc := NewFixedAssetService(&stubFixedAssetStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateAsset(context.Background(), dto.CreateFixedAssetRequest{
		AssetCategoryID: "cat-1", AssetCode: "AST-001", AssetName: "Laptop", AcquisitionDate: "2025-01-15",
		AcquisitionCost: "1000000", SalvageValue: "1000000", UsefulLifeMonths: 36, ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error when salvage_value >= acquisition_cost, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

func TestCreateAssetRejectsZeroUsefulLife(t *testing.T) {
	svc := NewFixedAssetService(&stubFixedAssetStore{}, stubEntryNumberGenerator{})

	_, err := svc.CreateAsset(context.Background(), dto.CreateFixedAssetRequest{
		AssetCategoryID: "cat-1", AssetCode: "AST-001", AssetName: "Laptop", AcquisitionDate: "2025-01-15",
		AcquisitionCost: "1000000", UsefulLifeMonths: 0, ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected an error for zero useful_life_months, got nil")
	}
	if code := appErrorCode(t, err); code != "FINANCE_VALIDATION_ERROR" {
		t.Fatalf("expected FINANCE_VALIDATION_ERROR, got %s", code)
	}
}

// TestGenerateStraightLineScheduleSumsExactly guards the rounding technique:
// 1,000,000 / 3 does not divide evenly, so naive per-period rounding would
// drift the schedule total away from the depreciable base. The
// cumulative-then-subtract method must absorb the whole remainder into a
// single period instead, keeping the sum exact.
func TestGenerateStraightLineScheduleSumsExactly(t *testing.T) {
	acquisitionDate := time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC)
	cost := big.NewRat(1000000, 1)
	salvage := big.NewRat(0, 1)

	schedule := generateStraightLineSchedule(acquisitionDate, cost, salvage, 3)

	if len(schedule) != 3 {
		t.Fatalf("expected 3 schedule rows, got %d", len(schedule))
	}

	expected := []string{"333333.33", "333333.34", "333333.33"}
	total := zeroRat()
	for i, row := range schedule {
		if row.Amount != expected[i] {
			t.Fatalf("period %d: expected amount %s, got %s", i+1, expected[i], row.Amount)
		}
		amount, err := moneyRat(row.Amount)
		if err != nil {
			t.Fatalf("period %d: invalid amount %s: %v", i+1, row.Amount, err)
		}
		total.Add(total, amount)
	}

	if formatMoney(total) != "1000000.00" {
		t.Fatalf("expected schedule to sum to exactly 1000000.00, got %s", formatMoney(total))
	}
}

// TestGenerateStraightLineScheduleCountsAcquisitionMonthInFull checks the
// documented assumption: the first period lands in the acquisition month
// itself, not the following month.
func TestGenerateStraightLineScheduleCountsAcquisitionMonthInFull(t *testing.T) {
	acquisitionDate := time.Date(2025, time.March, 20, 0, 0, 0, 0, time.UTC)
	schedule := generateStraightLineSchedule(acquisitionDate, big.NewRat(1200000, 1), big.NewRat(0, 1), 12)

	firstPeriod := schedule[0].PeriodDate
	if firstPeriod.Year() != 2025 || firstPeriod.Month() != time.March {
		t.Fatalf("expected first period to land in March 2025, got %s", firstPeriod.Format("2006-01-02"))
	}
	if firstPeriod.Day() != 31 {
		t.Fatalf("expected first period date to be the last day of March, got day %d", firstPeriod.Day())
	}
}

func TestCreateAssetPropagatesStoreError(t *testing.T) {
	store := &stubFixedAssetStore{createAssetErr: errFixedAssetStoreFailure}
	svc := NewFixedAssetService(store, stubEntryNumberGenerator{})

	_, err := svc.CreateAsset(context.Background(), dto.CreateFixedAssetRequest{
		AssetCategoryID: "cat-1", AssetCode: "AST-001", AssetName: "Laptop", AcquisitionDate: "2025-01-15",
		AcquisitionCost: "1000000", UsefulLifeMonths: 36, ContraAccountID: "acc-1",
	}, "user-1")

	if err == nil {
		t.Fatal("expected the store error to propagate, got nil")
	}
}

var errFixedAssetStoreFailure = &fixedAssetStoreFailure{}

type fixedAssetStoreFailure struct{}

func (e *fixedAssetStoreFailure) Error() string { return "store failure" }
