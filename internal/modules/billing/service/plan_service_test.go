package service

import (
	"context"
	"errors"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"

	"github.com/jackc/pgx/v5"
)

type stubPlanStore struct {
	plan                 model.Plan
	price                model.PlanPrice
	prices               []model.PlanPrice
	findPlanErr          error
	findPriceErr         error
	createPriceErr       error
	updatePriceErr       error
	deletePriceErr       error
	lastCreatePrice      repository.CreatePlanPriceParams
	lastUpdatePrice      repository.UpdatePlanPriceParams
	lastDeletePlanID     string
	lastDeletePriceID    string
	lastListPlanID       string
	lastListIncludeTrash bool
}

func (s *stubPlanStore) Create(context.Context, repository.CreatePlanParams) (model.Plan, error) {
	return s.plan, nil
}

func (s *stubPlanStore) FindByID(context.Context, string) (model.Plan, error) {
	if s.findPlanErr != nil {
		return model.Plan{}, s.findPlanErr
	}
	return s.plan, nil
}

func (s *stubPlanStore) FindByCode(context.Context, string) (model.Plan, error) {
	return s.plan, nil
}

func (s *stubPlanStore) List(context.Context, repository.PlanListFilter) ([]model.Plan, int64, error) {
	return []model.Plan{s.plan}, 1, nil
}

func (s *stubPlanStore) Update(context.Context, repository.UpdatePlanParams) (model.Plan, error) {
	return s.plan, nil
}

func (s *stubPlanStore) SoftDelete(context.Context, string) error {
	return nil
}

func (s *stubPlanStore) UpsertPrice(context.Context, repository.UpsertPlanPriceParams) (model.PlanPrice, error) {
	return s.price, nil
}

func (s *stubPlanStore) CreatePrice(
	_ context.Context,
	params repository.CreatePlanPriceParams,
) (model.PlanPrice, error) {
	s.lastCreatePrice = params
	if s.createPriceErr != nil {
		return model.PlanPrice{}, s.createPriceErr
	}
	return s.price, nil
}

func (s *stubPlanStore) ListPrices(
	_ context.Context,
	planID string,
	includeDeleted bool,
) ([]model.PlanPrice, error) {
	s.lastListPlanID = planID
	s.lastListIncludeTrash = includeDeleted
	return s.prices, nil
}

func (s *stubPlanStore) FindPriceByID(
	context.Context,
	string,
	string,
	bool,
) (model.PlanPrice, error) {
	if s.findPriceErr != nil {
		return model.PlanPrice{}, s.findPriceErr
	}
	return s.price, nil
}

func (s *stubPlanStore) UpdatePrice(
	_ context.Context,
	params repository.UpdatePlanPriceParams,
) (model.PlanPrice, error) {
	s.lastUpdatePrice = params
	if s.updatePriceErr != nil {
		return model.PlanPrice{}, s.updatePriceErr
	}
	return s.price, nil
}

func (s *stubPlanStore) SoftDeletePrice(_ context.Context, planID string, priceID string) error {
	s.lastDeletePlanID = planID
	s.lastDeletePriceID = priceID
	return s.deletePriceErr
}

func TestPlanServiceCreatePrice(t *testing.T) {
	store := &stubPlanStore{
		plan: model.Plan{ID: "plan-1", Code: "growth"},
		price: model.PlanPrice{
			ID:              "price-1",
			PlanID:          "plan-1",
			BillingInterval: model.BillingIntervalMonthly,
			Currency:        "IDR",
			Amount:          "199000.00",
			IsActive:        true,
		},
	}
	service := NewPlanService(store)

	response, err := service.CreatePrice(context.Background(), "plan-1", dto.CreatePlanPriceRequest{
		BillingInterval: "monthly",
		Currency:        "idr",
		Amount:          "199000",
	})
	if err != nil {
		t.Fatalf("CreatePrice error = %v", err)
	}
	if response.ID != "price-1" || response.BillingInterval != "monthly" {
		t.Fatalf("response = %#v", response)
	}
	if store.lastCreatePrice.PlanID != "plan-1" ||
		store.lastCreatePrice.BillingInterval != model.BillingIntervalMonthly ||
		store.lastCreatePrice.Currency != "idr" ||
		store.lastCreatePrice.Amount != "199000" {
		t.Fatalf("lastCreatePrice = %#v", store.lastCreatePrice)
	}
}

func TestPlanServiceUpdatePriceMapsNotFound(t *testing.T) {
	store := &stubPlanStore{
		plan:         model.Plan{ID: "plan-1", Code: "growth"},
		findPriceErr: pgx.ErrNoRows,
	}
	service := NewPlanService(store)
	amount := "299000.00"

	_, err := service.UpdatePrice(context.Background(), "plan-1", "price-missing", dto.UpdatePlanPriceRequest{
		Amount: &amount,
	})
	if err == nil {
		t.Fatal("UpdatePrice error = nil, want not found")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want AppError", err)
	}
	if appErr.Code != billing.ErrCodePlanPriceNotFound {
		t.Fatalf("error code = %s, want %s", appErr.Code, billing.ErrCodePlanPriceNotFound)
	}
}

func TestPlanServiceDeletePriceUsesPlanAndPriceScope(t *testing.T) {
	store := &stubPlanStore{
		plan:  model.Plan{ID: "plan-1", Code: "growth"},
		price: model.PlanPrice{ID: "price-1", PlanID: "plan-1"},
	}
	service := NewPlanService(store)

	if err := service.DeletePrice(context.Background(), "plan-1", "price-1"); err != nil {
		t.Fatalf("DeletePrice error = %v", err)
	}
	if store.lastDeletePlanID != "plan-1" || store.lastDeletePriceID != "price-1" {
		t.Fatalf("delete scope = %s/%s", store.lastDeletePlanID, store.lastDeletePriceID)
	}
}

func TestPlanServiceListPricesRequiresPlan(t *testing.T) {
	store := &stubPlanStore{
		findPlanErr: pgx.ErrNoRows,
	}
	service := NewPlanService(store)

	_, err := service.ListPrices(context.Background(), "missing-plan", false)
	if err == nil {
		t.Fatal("ListPrices error = nil, want plan not found")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want AppError", err)
	}
	if appErr.Code != billing.ErrCodePlanNotFound {
		t.Fatalf("error code = %s, want %s", appErr.Code, billing.ErrCodePlanNotFound)
	}
}
