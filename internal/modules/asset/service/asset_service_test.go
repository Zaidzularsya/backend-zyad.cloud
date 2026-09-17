package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
	"zyad.cloud/internal/modules/asset/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/storage"
)

const assetServiceOrganizationID = "11111111-1111-1111-1111-111111111111"

func mustAssetScope(t *testing.T) coretenant.Scope {
	t.Helper()
	scope, err := ScopeForOrganization(assetServiceOrganizationID)
	if err != nil {
		t.Fatalf("ScopeForOrganization() error = %v", err)
	}
	return scope
}

type assetRepoStub struct {
	createCalled bool
	createParams repository.CreateAssetObjectParams
	createResult domain.AssetObject
	createErr    error
	getResult    domain.AssetObject
	getErr       error
	deleteCalled bool
	deleteErr    error
	sumSizeBytes int64
	sumSizeErr   error
}

func (s *assetRepoStub) Create(_ context.Context, _ coretenant.Scope, params repository.CreateAssetObjectParams) (domain.AssetObject, error) {
	s.createCalled = true
	s.createParams = params
	if s.createErr != nil {
		return domain.AssetObject{}, s.createErr
	}
	return s.createResult, nil
}

func (s *assetRepoStub) Get(context.Context, coretenant.Scope, string) (domain.AssetObject, error) {
	return s.getResult, s.getErr
}

func (s *assetRepoStub) List(context.Context, coretenant.Scope) ([]domain.AssetObject, error) {
	return nil, nil
}

func (s *assetRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	s.deleteCalled = true
	return s.deleteErr
}

func (s *assetRepoStub) SumSizeBytes(context.Context, coretenant.Scope) (int64, error) {
	return s.sumSizeBytes, s.sumSizeErr
}

type landingMediaSumStub struct {
	total int64
	err   error
}

func (s *landingMediaSumStub) SumSizeBytes(context.Context, coretenant.Scope) (int64, error) {
	return s.total, s.err
}

type entitlementFinderStub struct {
	entitlement organizationmodel.Entitlement
	err         error
}

func (s *entitlementFinderStub) FindEffective(context.Context, string, string, time.Time) (organizationmodel.Entitlement, error) {
	return s.entitlement, s.err
}

type objectStoragePutStub struct {
	putCalled    bool
	deleteCalled bool
	putErr       error
}

func (s *objectStoragePutStub) Put(context.Context, string, io.Reader, string, int64) error {
	s.putCalled = true
	return s.putErr
}

func (s *objectStoragePutStub) Delete(context.Context, string) error {
	s.deleteCalled = true
	return nil
}

func TestAssetServiceUploadObjectAllowsWhenNoQuotaConfigured(t *testing.T) {
	repo := &assetRepoStub{createResult: domain.AssetObject{ID: "asset-1"}}
	store := &objectStoragePutStub{}
	finder := &entitlementFinderStub{err: pgx.ErrNoRows}
	svc := NewAssetService(repo, &landingMediaSumStub{}, WithAssetObjectStorage(store), WithAssetEntitlementFinder(finder))

	_, err := svc.UploadObject(context.Background(), mustAssetScope(t), UploadObjectParams{
		Filename:  "ktp.png",
		MimeType:  "image/png",
		SizeBytes: 1024,
		Class:     domain.ObjectClassPrivate,
	}, bytes.NewReader(make([]byte, 1024)))
	if err != nil {
		t.Fatalf("UploadObject() error = %v, want nil when no entitlement row exists (unlimited)", err)
	}
	if !repo.createCalled || !store.putCalled {
		t.Fatal("UploadObject() should persist and store the object when quota allows")
	}
}

func TestAssetServiceUploadObjectRejectsWhenOverQuota(t *testing.T) {
	repo := &assetRepoStub{sumSizeBytes: 900}
	store := &objectStoragePutStub{}
	finder := &entitlementFinderStub{entitlement: organizationmodel.Entitlement{
		Limits: map[string]any{"limit": int64(1000)},
	}}
	svc := NewAssetService(repo, &landingMediaSumStub{total: 50}, WithAssetObjectStorage(store), WithAssetEntitlementFinder(finder))

	_, err := svc.UploadObject(context.Background(), mustAssetScope(t), UploadObjectParams{
		Filename:  "big.pdf",
		MimeType:  "application/pdf",
		SizeBytes: 200, // 900 (asset) + 50 (landing) + 200 = 1150 > 1000 limit
		Class:     domain.ObjectClassPrivate,
	}, bytes.NewReader(make([]byte, 200)))
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("UploadObject() error = %v, want %v", err, ErrQuotaExceeded)
	}
	if repo.createCalled || store.putCalled {
		t.Fatal("UploadObject() must not persist or store the object when quota is exceeded")
	}
}

func TestAssetServiceUploadObjectAllowsWhenWithinQuota(t *testing.T) {
	repo := &assetRepoStub{sumSizeBytes: 500, createResult: domain.AssetObject{ID: "asset-1"}}
	store := &objectStoragePutStub{}
	finder := &entitlementFinderStub{entitlement: organizationmodel.Entitlement{
		Limits: map[string]any{"limit": int64(1000)},
	}}
	svc := NewAssetService(repo, &landingMediaSumStub{total: 100}, WithAssetObjectStorage(store), WithAssetEntitlementFinder(finder))

	_, err := svc.UploadObject(context.Background(), mustAssetScope(t), UploadObjectParams{
		Filename:  "small.pdf",
		MimeType:  "application/pdf",
		SizeBytes: 200, // 500 + 100 + 200 = 800 <= 1000
		Class:     domain.ObjectClassPrivate,
	}, bytes.NewReader(make([]byte, 200)))
	if err != nil {
		t.Fatalf("UploadObject() error = %v, want nil when usage stays within limit", err)
	}
	if !repo.createCalled || !store.putCalled {
		t.Fatal("UploadObject() should persist and store the object when quota allows")
	}
}

func TestAssetServiceUploadObjectRejectsInvalidMimeType(t *testing.T) {
	repo := &assetRepoStub{}
	store := &objectStoragePutStub{}
	svc := NewAssetService(repo, &landingMediaSumStub{}, WithAssetObjectStorage(store))

	_, err := svc.UploadObject(context.Background(), mustAssetScope(t), UploadObjectParams{
		Filename:  "malware.exe",
		MimeType:  "application/x-msdownload",
		SizeBytes: 10,
	}, bytes.NewReader(make([]byte, 10)))
	if !errors.Is(err, ErrInvalidMimeType) {
		t.Fatalf("UploadObject() error = %v, want %v", err, ErrInvalidMimeType)
	}
	if repo.createCalled || store.putCalled {
		t.Fatal("UploadObject() must not reach storage/repo for a disallowed mime type")
	}
}

func TestAssetServiceUploadObjectRejectsFileTooLarge(t *testing.T) {
	repo := &assetRepoStub{}
	store := &objectStoragePutStub{}
	svc := NewAssetService(repo, &landingMediaSumStub{}, WithAssetObjectStorage(store))

	_, err := svc.UploadObject(context.Background(), mustAssetScope(t), UploadObjectParams{
		Filename:  "huge.png",
		MimeType:  "image/png",
		SizeBytes: MaxObjectSizeBytes + 1,
	}, bytes.NewReader(make([]byte, 10)))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("UploadObject() error = %v, want %v", err, ErrFileTooLarge)
	}
	if repo.createCalled || store.putCalled {
		t.Fatal("UploadObject() must not reach storage/repo for an oversized file")
	}
}

func TestAssetServiceGetUsageSumsAcrossBothTables(t *testing.T) {
	repo := &assetRepoStub{sumSizeBytes: 300}
	finder := &entitlementFinderStub{entitlement: organizationmodel.Entitlement{
		Limits: map[string]any{"limit": int64(1000)},
	}}
	svc := NewAssetService(repo, &landingMediaSumStub{total: 700}, WithAssetEntitlementFinder(finder))

	usage, err := svc.GetUsage(context.Background(), mustAssetScope(t))
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if usage.UsedBytes != 1000 {
		t.Fatalf("GetUsage().UsedBytes = %d, want 1000 (300 asset + 700 landing)", usage.UsedBytes)
	}
	if usage.LimitBytes == nil || *usage.LimitBytes != 1000 {
		t.Fatalf("GetUsage().LimitBytes = %#v, want 1000", usage.LimitBytes)
	}
}

var _ storage.ObjectStorage = (*objectStoragePutStub)(nil)
