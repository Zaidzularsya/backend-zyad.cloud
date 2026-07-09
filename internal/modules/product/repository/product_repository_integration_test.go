//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestProductRepositoryPlanFeatureEntitlementIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	planRepo := NewPlanRepository(db)
	featureRepo := NewFeatureRepository(db)
	entitlementRepo := NewPlanEntitlementRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	plans, total, err := planRepo.List(ctx, PlanListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("List plans error = %v", err)
	}
	if total < 5 || len(plans) < 5 {
		t.Fatalf("List plans total = %d len = %d, want seeded plans", total, len(plans))
	}

	growth, err := planRepo.FindByCode(ctx, "growth")
	if err != nil {
		t.Fatalf("FindByCode(growth) error = %v", err)
	}
	if growth.Code != "growth" || !growth.IsAvailable() {
		t.Fatalf("FindByCode(growth) = %#v", growth)
	}

	landingFeature, err := featureRepo.FindByKey(ctx, "landing.enabled")
	if err != nil {
		t.Fatalf("FindByKey(landing.enabled) error = %v", err)
	}
	if landingFeature.ValueType != model.FeatureValueTypeBoolean {
		t.Fatalf("landing.enabled ValueType = %s", landingFeature.ValueType)
	}

	features, featureTotal, err := featureRepo.List(ctx, FeatureListFilter{
		Module: "landing",
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("List landing features error = %v", err)
	}
	if featureTotal < 6 || len(features) < 6 {
		t.Fatalf("List landing features total = %d len = %d, want seeded landing features", featureTotal, len(features))
	}

	prices, err := planRepo.ListPrices(ctx, growth.ID, false)
	if err != nil {
		t.Fatalf("ListPrices(growth) error = %v", err)
	}
	if len(prices) == 0 {
		t.Fatal("ListPrices(growth) returned no seeded prices")
	}

	seededEntitlements, err := entitlementRepo.ListByPlanID(ctx, growth.ID)
	if err != nil {
		t.Fatalf("ListByPlanID(growth) error = %v", err)
	}
	if len(seededEntitlements) == 0 {
		t.Fatal("ListByPlanID(growth) returned no seeded entitlements")
	}

	testCode := strings.ReplaceAll("repo_"+testutil.UniqueCode("product"), ".", "_")
	testPlan, err := planRepo.Create(ctx, CreatePlanParams{
		Code:      testCode,
		Name:      "Repository Test Plan",
		Type:      model.PlanTypePaid,
		IsPublic:  false,
		IsActive:  true,
		SortOrder: 999,
	})
	if err != nil {
		t.Fatalf("Create test plan error = %v", err)
	}
	t.Cleanup(func() {
		_ = planRepo.SoftDelete(context.Background(), testPlan.ID)
	})

	price, err := planRepo.UpsertPrice(ctx, UpsertPlanPriceParams{
		PlanID:          testPlan.ID,
		BillingInterval: model.BillingIntervalMonthly,
		Currency:        "IDR",
		Amount:          "12345.67",
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("UpsertPrice error = %v", err)
	}
	if price.Amount != "12345.67" {
		t.Fatalf("UpsertPrice Amount = %s", price.Amount)
	}

	createdYearlyPrice, err := planRepo.CreatePrice(ctx, CreatePlanPriceParams{
		PlanID:          testPlan.ID,
		BillingInterval: model.BillingIntervalYearly,
		Currency:        "IDR",
		Amount:          "120000.00",
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("CreatePrice error = %v", err)
	}
	if createdYearlyPrice.BillingInterval != model.BillingIntervalYearly {
		t.Fatalf("CreatePrice interval = %s", createdYearlyPrice.BillingInterval)
	}

	foundPrice, err := planRepo.FindPriceByID(ctx, testPlan.ID, createdYearlyPrice.ID, false)
	if err != nil {
		t.Fatalf("FindPriceByID error = %v", err)
	}
	if foundPrice.ID != createdYearlyPrice.ID {
		t.Fatalf("FindPriceByID ID = %s, want %s", foundPrice.ID, createdYearlyPrice.ID)
	}

	updatedPrice, err := planRepo.UpdatePrice(ctx, UpdatePlanPriceParams{
		ID:       createdYearlyPrice.ID,
		PlanID:   testPlan.ID,
		Amount:   stringPointer("150000.00"),
		IsActive: boolPointer(false),
	})
	if err != nil {
		t.Fatalf("UpdatePrice error = %v", err)
	}
	if updatedPrice.Amount != "150000.00" || updatedPrice.IsActive {
		t.Fatalf("UpdatePrice = %#v", updatedPrice)
	}

	if err := planRepo.SoftDeletePrice(ctx, testPlan.ID, createdYearlyPrice.ID); err != nil {
		t.Fatalf("SoftDeletePrice error = %v", err)
	}
	deletedPrices, err := planRepo.ListPrices(ctx, testPlan.ID, true)
	if err != nil {
		t.Fatalf("ListPrices(includeDeleted) error = %v", err)
	}
	deletedFound := false
	for _, item := range deletedPrices {
		if item.ID == createdYearlyPrice.ID && item.DeletedAt != nil {
			deletedFound = true
			break
		}
	}
	if !deletedFound {
		t.Fatalf("deleted price %s not found in includeDeleted list", createdYearlyPrice.ID)
	}

	enabled := true
	entitlements, err := entitlementRepo.ReplaceByPlanID(ctx, testPlan.ID, []UpsertPlanEntitlementParams{
		{
			FeatureID: landingFeature.ID,
			ValueBool: &enabled,
		},
	})
	if err != nil {
		t.Fatalf("ReplaceByPlanID error = %v", err)
	}
	if len(entitlements) != 1 || entitlements[0].FeatureKey != "landing.enabled" {
		t.Fatalf("ReplaceByPlanID entitlements = %#v", entitlements)
	}
	if entitlements[0].ValueBool == nil || !*entitlements[0].ValueBool {
		t.Fatalf("ReplaceByPlanID ValueBool = %#v", entitlements[0].ValueBool)
	}
}

func stringPointer(value string) *string {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}
