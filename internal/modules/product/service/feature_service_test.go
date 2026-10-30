package service

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/modules/product/repository"
)

type fakeFeatureStore struct {
	created    repository.CreateFeatureParams
	updated    repository.UpdateFeatureParams
	existing   model.Feature
	createCall int
	updateCall int
}

func (f *fakeFeatureStore) Create(_ context.Context, p repository.CreateFeatureParams) (model.Feature, error) {
	f.createCall++
	f.created = p
	return model.Feature{Key: p.Key, ValueType: p.ValueType, TenantOfferable: p.TenantOfferable, TenantMax: p.TenantMax}, nil
}

func (f *fakeFeatureStore) FindByID(context.Context, string) (model.Feature, error) {
	return f.existing, nil
}

func (f *fakeFeatureStore) FindByKey(context.Context, string) (model.Feature, error) {
	return f.existing, nil
}

func (f *fakeFeatureStore) List(context.Context, repository.FeatureListFilter) ([]model.Feature, int64, error) {
	return nil, 0, nil
}

func (f *fakeFeatureStore) Update(_ context.Context, p repository.UpdateFeatureParams) (model.Feature, error) {
	f.updateCall++
	f.updated = p
	return model.Feature{TenantOfferable: p.TenantOfferable, TenantMax: p.TenantMax}, nil
}

func intPtr(n int) *int { return &n }

func createReq(valueType string, offerable bool, max *int) dto.CreateFeatureRequest {
	return dto.CreateFeatureRequest{
		FeatureKey: "x.y", Module: "x", Name: "X", ValueType: valueType,
		TenantOfferable: offerable, TenantMax: max,
	}
}

func TestFeatureService_Create_IntegerOfferableRequiresMax(t *testing.T) {
	store := &fakeFeatureStore{}
	svc := NewFeatureService(store)
	if _, err := svc.Create(context.Background(), createReq("integer", true, nil)); err == nil {
		t.Fatal("want validation error for integer offerable without tenant_max")
	}
	if store.createCall != 0 {
		t.Fatal("store must not be called on validation error")
	}
	if _, err := svc.Create(context.Background(), createReq("integer", true, intPtr(0))); err == nil {
		t.Fatal("want validation error for tenant_max <= 0")
	}
	if _, err := svc.Create(context.Background(), createReq("integer", true, intPtr(5))); err != nil {
		t.Fatalf("integer offerable with max must pass: %v", err)
	}
	if store.created.TenantMax == nil || *store.created.TenantMax != 5 {
		t.Fatalf("tenant_max not forwarded: %+v", store.created.TenantMax)
	}
}

func TestFeatureService_Create_BooleanOfferableIgnoresMax(t *testing.T) {
	store := &fakeFeatureStore{}
	svc := NewFeatureService(store)
	if _, err := svc.Create(context.Background(), createReq("boolean", true, nil)); err != nil {
		t.Fatalf("boolean offerable without max must pass: %v", err)
	}
	if !store.created.TenantOfferable || store.created.TenantMax != nil {
		t.Fatalf("unexpected params: %+v", store.created)
	}
	if _, err := svc.Create(context.Background(), createReq("boolean", true, intPtr(3))); err != nil {
		t.Fatalf("boolean with stray max must be accepted and nulled: %v", err)
	}
	if store.created.TenantMax != nil {
		t.Fatalf("tenant_max must be nulled for boolean, got %d", *store.created.TenantMax)
	}
}

func TestFeatureService_Update_RevalidatesAgainstExisting(t *testing.T) {
	store := &fakeFeatureStore{existing: model.Feature{ID: "id", ValueType: model.FeatureValueTypeInteger}}
	svc := NewFeatureService(store)
	yes := true
	if _, err := svc.Update(context.Background(), "id", dto.UpdateFeatureRequest{TenantOfferable: &yes}); err == nil {
		t.Fatal("making integer feature offerable without max must fail")
	}
	if _, err := svc.Update(context.Background(), "id", dto.UpdateFeatureRequest{TenantOfferable: &yes, TenantMax: intPtr(10)}); err != nil {
		t.Fatalf("offerable with max must pass: %v", err)
	}
	if !store.updated.TenantOfferable || store.updated.TenantMax == nil || *store.updated.TenantMax != 10 {
		t.Fatalf("unexpected update params: %+v", store.updated)
	}
	// Ubah tipe ke boolean: max harus di-null.
	store.existing = model.Feature{ID: "id", ValueType: model.FeatureValueTypeInteger, TenantOfferable: true, TenantMax: intPtr(10)}
	vt := "boolean"
	if _, err := svc.Update(context.Background(), "id", dto.UpdateFeatureRequest{ValueType: &vt}); err != nil {
		t.Fatalf("type change to boolean must pass: %v", err)
	}
	if store.updated.TenantMax != nil {
		t.Fatalf("tenant_max must be nulled after change to boolean")
	}
}
