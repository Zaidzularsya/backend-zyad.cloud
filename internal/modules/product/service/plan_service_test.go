package service

import (
	"context"
	"errors"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	product "zyad.cloud/internal/modules/product"
	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/modules/product/repository"

	"github.com/jackc/pgx/v5"
)

type stubPlanBenefitStore struct {
	benefits []repository.PublicEntitlement
}

func (s *stubPlanBenefitStore) ListActiveByPlanID(context.Context, string) ([]repository.PublicEntitlement, error) {
	return s.benefits, nil
}

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
	lastListFilter       repository.PlanListFilter
	plans                []model.Plan
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

func (s *stubPlanStore) List(_ context.Context, filter repository.PlanListFilter) ([]model.Plan, int64, error) {
	s.lastListFilter = filter
	if s.plans != nil {
		return s.plans, int64(len(s.plans)), nil
	}
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
	service := NewPlanService(store, &stubPlanBenefitStore{})

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
	service := NewPlanService(store, &stubPlanBenefitStore{})
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
	if appErr.Code != product.ErrCodePlanPriceNotFound {
		t.Fatalf("error code = %s, want %s", appErr.Code, product.ErrCodePlanPriceNotFound)
	}
}

func TestPlanServiceDeletePriceUsesPlanAndPriceScope(t *testing.T) {
	store := &stubPlanStore{
		plan:  model.Plan{ID: "plan-1", Code: "growth"},
		price: model.PlanPrice{ID: "price-1", PlanID: "plan-1"},
	}
	service := NewPlanService(store, &stubPlanBenefitStore{})

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
	service := NewPlanService(store, &stubPlanBenefitStore{})

	_, err := service.ListPrices(context.Background(), "missing-plan", false)
	if err == nil {
		t.Fatal("ListPrices error = nil, want plan not found")
	}
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want AppError", err)
	}
	if appErr.Code != product.ErrCodePlanNotFound {
		t.Fatalf("error code = %s, want %s", appErr.Code, product.ErrCodePlanNotFound)
	}
}

func TestPlanServiceListPublicFiltersPublicActive(t *testing.T) {
	store := &stubPlanStore{
		plans: []model.Plan{
			{ID: "plan-1", Code: "growth", Name: "Growth", SortOrder: 1},
		},
		prices: []model.PlanPrice{
			{ID: "price-1", PlanID: "plan-1", BillingInterval: model.BillingIntervalMonthly, Currency: "IDR", Amount: "199000.00", IsActive: true},
			{ID: "price-2", PlanID: "plan-1", BillingInterval: model.BillingIntervalYearly, Currency: "IDR", Amount: "1990000.00", IsActive: false},
		},
	}
	trueVal := true
	falseVal := false
	maxPages := int64(10)
	benefitStore := &stubPlanBenefitStore{
		benefits: []repository.PublicEntitlement{
			{FeatureKey: "landing.enabled", FeatureName: "Landing page module", ValueType: model.FeatureValueTypeBoolean, ValueBool: &trueVal},
			{FeatureKey: "crm.enabled", FeatureName: "CRM module", ValueType: model.FeatureValueTypeBoolean, ValueBool: &falseVal},
			{FeatureKey: "landing.max_pages", FeatureName: "Maximum landing pages", Unit: "page", ValueType: model.FeatureValueTypeInteger, ValueInt: &maxPages},
		},
	}
	service := NewPlanService(store, benefitStore)

	response, err := service.ListPublic(context.Background())
	if err != nil {
		t.Fatalf("ListPublic error = %v", err)
	}

	if store.lastListFilter.IsPublic == nil || !*store.lastListFilter.IsPublic {
		t.Fatalf("lastListFilter.IsPublic = %#v, want true", store.lastListFilter.IsPublic)
	}
	if store.lastListFilter.IsActive == nil || !*store.lastListFilter.IsActive {
		t.Fatalf("lastListFilter.IsActive = %#v, want true", store.lastListFilter.IsActive)
	}

	if len(response.Items) != 1 {
		t.Fatalf("Items = %#v, want 1 item", response.Items)
	}
	item := response.Items[0]
	if item.Code != "growth" || item.Name != "Growth" || item.SortOrder != 1 {
		t.Fatalf("item = %#v", item)
	}
	if len(item.Prices) != 1 || item.Prices[0].BillingInterval != "monthly" {
		t.Fatalf("item.Prices = %#v, want only the active monthly price", item.Prices)
	}
	if len(item.Benefits) != 2 {
		t.Fatalf("item.Benefits = %#v, want 2 benefits (false boolean excluded)", item.Benefits)
	}
	if item.Benefits[0].Label != "Landing page module" || item.Benefits[0].Value != "" {
		t.Fatalf("Benefits[0] = %#v", item.Benefits[0])
	}
	if item.Benefits[1].Label != "Maximum landing pages" || item.Benefits[1].Value != "10 page" {
		t.Fatalf("Benefits[1] = %#v", item.Benefits[1])
	}
}
