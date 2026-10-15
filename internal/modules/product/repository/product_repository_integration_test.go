//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestProductRepositoryFeatureIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	featureRepo := NewFeatureRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
}
