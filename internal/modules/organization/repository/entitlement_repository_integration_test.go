//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestEntitlementRepositoryPrecedenceVersionAndUsageIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	organizationRepo := NewOrganizationRepository(db)
	entitlementRepo := NewEntitlementRepository(db)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("entitlement"), ".", "-")

	organization, err := organizationRepo.Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "entitlement-" + suffix,
		Name:          "Entitlement Repository Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	var actorUserID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Entitlement Actor', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("entitlement-%s@example.test", suffix)).Scan(&actorUserID); err != nil {
		t.Fatalf("create actor user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_usage_counters WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_entitlements WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, actorUserID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	featureKey := "landing.max_pages"
	plan, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID:  organization.ID,
		FeatureKey:      " Landing.Max_Pages ",
		Source:          model.EntitlementSourcePlan,
		SourceReference: "plan-basic",
		Limits:          map[string]any{"value": 5},
		EffectiveFrom:   now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("upsert plan: %v", err)
	}
	trialUntil := now.Add(24 * time.Hour)
	if _, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID:  organization.ID,
		FeatureKey:      featureKey,
		Source:          model.EntitlementSourceTrial,
		SourceReference: "trial-onboarding",
		Limits:          map[string]any{"value": 10},
		EffectiveFrom:   now.Add(-time.Hour),
		EffectiveUntil:  &trialUntil,
	}); err != nil {
		t.Fatalf("upsert trial: %v", err)
	}
	if _, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID:  organization.ID,
		FeatureKey:      featureKey,
		Source:          model.EntitlementSourceAddon,
		SourceReference: "addon-pages",
		Limits:          map[string]any{"value": 20},
		EffectiveFrom:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("upsert addon: %v", err)
	}
	overrideUntil := now.Add(time.Hour)
	override, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID: organization.ID,
		FeatureKey:     featureKey,
		Source:         model.EntitlementSourcePlatformOverride,
		Limits:         map[string]any{"value": 1},
		EffectiveFrom:  now.Add(-time.Minute),
		EffectiveUntil: &overrideUntil,
		Reason:         "security restriction",
		ActorUserID:    actorUserID,
	})
	if err != nil {
		t.Fatalf("upsert override: %v", err)
	}

	effective, err := entitlementRepo.FindEffective(ctx, organization.ID, featureKey, now)
	if err != nil {
		t.Fatalf("FindEffective() error = %v", err)
	}
	if effective.ID != override.ID || effective.Source != model.EntitlementSourcePlatformOverride {
		t.Fatalf("FindEffective() = %#v, want platform override", effective)
	}

	overrideUpdated, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID: organization.ID,
		FeatureKey:     featureKey,
		Source:         model.EntitlementSourcePlatformOverride,
		Limits:         map[string]any{"value": 2},
		EffectiveFrom:  now.Add(-time.Minute),
		EffectiveUntil: &overrideUntil,
		Reason:         "adjusted security restriction",
		ActorUserID:    actorUserID,
	})
	if err != nil {
		t.Fatalf("update override: %v", err)
	}
	if overrideUpdated.ID != override.ID || overrideUpdated.Version <= override.Version {
		t.Fatalf("updated override = %#v, original = %#v", overrideUpdated, override)
	}

	metadata, err := entitlementRepo.VersionMetadata(ctx, organization.ID)
	if err != nil {
		t.Fatalf("VersionMetadata() error = %v", err)
	}
	if metadata.RowCount != 4 || metadata.MaxVersion != overrideUpdated.Version ||
		metadata.LatestUpdatedAt == nil {
		t.Fatalf("VersionMetadata() = %#v", metadata)
	}

	inactiveOverride, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID: organization.ID,
		FeatureKey:     featureKey,
		Source:         model.EntitlementSourcePlatformOverride,
		Status:         model.EntitlementStatusInactive,
		Limits:         map[string]any{"value": 2},
		EffectiveFrom:  now.Add(-time.Minute),
		EffectiveUntil: &overrideUntil,
		Reason:         "restriction removed",
		ActorUserID:    actorUserID,
	})
	if err != nil {
		t.Fatalf("deactivate override: %v", err)
	}
	if inactiveOverride.Status != model.EntitlementStatusInactive {
		t.Fatalf("inactive override status = %q", inactiveOverride.Status)
	}
	effective, err = entitlementRepo.FindEffective(ctx, organization.ID, featureKey, now)
	if err != nil {
		t.Fatalf("FindEffective() after override error = %v", err)
	}
	if effective.Source != model.EntitlementSourceAddon {
		t.Fatalf("FindEffective() source = %q, want addon", effective.Source)
	}

	if _, err := entitlementRepo.Upsert(ctx, UpsertEntitlementParams{
		OrganizationID:  organization.ID,
		FeatureKey:      featureKey,
		Source:          model.EntitlementSourceAddon,
		SourceReference: "addon-pages",
		Status:          model.EntitlementStatusInactive,
		Limits:          map[string]any{"value": 20},
		EffectiveFrom:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("deactivate addon: %v", err)
	}
	effective, err = entitlementRepo.FindEffective(ctx, organization.ID, featureKey, now)
	if err != nil {
		t.Fatalf("FindEffective() after addon error = %v", err)
	}
	if effective.Source != model.EntitlementSourceTrial {
		t.Fatalf("FindEffective() source = %q, want trial", effective.Source)
	}
	effective, err = entitlementRepo.FindEffective(ctx, organization.ID, featureKey, trialUntil)
	if err != nil {
		t.Fatalf("FindEffective() after trial expiry error = %v", err)
	}
	if effective.Source != model.EntitlementSourcePlan {
		t.Fatalf("FindEffective() source = %q, want plan", effective.Source)
	}

	entitlements, total, err := entitlementRepo.List(ctx, organization.ID, EntitlementListFilter{
		FeatureKey: featureKey,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 4 || len(entitlements) != 4 || plan.FeatureKey != featureKey {
		t.Fatalf("List() total = %d, entitlements = %#v", total, entitlements)
	}

	periodStart := now.Truncate(24 * time.Hour)
	usageKey := UsageCounterKey{
		OrganizationID: organization.ID,
		FeatureKey:     featureKey,
		MetricKey:      "page_count",
		PeriodStart:    periodStart,
		PeriodEnd:      periodStart.Add(24 * time.Hour),
	}
	maxUsage := int64(3)
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, incrementErr := entitlementRepo.IncrementUsage(
				context.Background(),
				usageKey,
				2,
				&maxUsage,
				time.Now().UTC(),
			)
			errs <- incrementErr
		}()
	}
	waitGroup.Wait()
	close(errs)

	var usageSucceeded int
	var usageRejected int
	for incrementErr := range errs {
		switch {
		case incrementErr == nil:
			usageSucceeded++
		case errors.Is(incrementErr, ErrUsageLimitExceeded):
			usageRejected++
		default:
			t.Fatalf("concurrent usage error = %v", incrementErr)
		}
	}
	if usageSucceeded != 1 || usageRejected != 1 {
		t.Fatalf("usage succeeded=%d rejected=%d, want 1 each", usageSucceeded, usageRejected)
	}

	counter, err := entitlementRepo.IncrementUsage(ctx, usageKey, 1, &maxUsage, time.Now().UTC())
	if err != nil {
		t.Fatalf("IncrementUsage() to limit error = %v", err)
	}
	if counter.UsageValue != maxUsage || counter.Version < 2 {
		t.Fatalf("usage counter = %#v", counter)
	}
	if _, err := entitlementRepo.IncrementUsage(
		ctx,
		usageKey,
		1,
		&maxUsage,
		time.Now().UTC(),
	); !errors.Is(err, ErrUsageLimitExceeded) {
		t.Fatalf("IncrementUsage() over limit error = %v", err)
	}

	storedCounter, err := entitlementRepo.GetUsage(ctx, usageKey)
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if storedCounter.UsageValue != maxUsage {
		t.Fatalf("GetUsage() value = %d, want %d", storedCounter.UsageValue, maxUsage)
	}
}
