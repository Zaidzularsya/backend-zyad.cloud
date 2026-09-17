package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
	"zyad.cloud/internal/modules/asset/repository"
	"zyad.cloud/internal/platform/storage"
)

var (
	ErrInvalidMimeType = errors.New("invalid mime type")
	ErrFileTooLarge    = errors.New("file too large")
	ErrStorageRequired = errors.New("asset storage is not configured")
	ErrQuotaExceeded   = errors.New("tenant storage quota exceeded")
)

// MaxObjectSizeBytes: batas per-file. Sama seperti landing media untuk saat
// ini (10MB) — dokumen KTP/legal pada umumnya jauh di bawah ini.
const MaxObjectSizeBytes = 10 * 1024 * 1024

const assetStorageNamespace = "tenant-storage"

// featureStorageMaxBytes adalah feature key entitlement untuk kuota storage
// tenant — konvensi sama seperti feature key lain di codebase ini (string
// lokal, bukan enum terpusat; lihat "users.invite_user" di membership_service.go).
const featureStorageMaxBytes = "storage.max_bytes"
const storageLimitKeyBytes = "limit"

var allowedObjectMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"application/pdf": true,
}

type assetService struct {
	assetRepo         repository.AssetRepository
	landingMediaRepo  LandingMediaSumRepository
	objectStorage     storage.ObjectStorage
	entitlementFinder EntitlementFinder
	logger            *slog.Logger
}

type AssetServiceOption func(*assetService)

func WithAssetObjectStorage(objectStorage storage.ObjectStorage) AssetServiceOption {
	return func(s *assetService) {
		s.objectStorage = objectStorage
	}
}

func WithAssetEntitlementFinder(finder EntitlementFinder) AssetServiceOption {
	return func(s *assetService) {
		s.entitlementFinder = finder
	}
}

func WithAssetLogger(logger *slog.Logger) AssetServiceOption {
	return func(s *assetService) {
		s.logger = logger
	}
}

func NewAssetService(
	assetRepo repository.AssetRepository,
	landingMediaRepo LandingMediaSumRepository,
	options ...AssetServiceOption,
) AssetService {
	service := &assetService{
		assetRepo:        assetRepo,
		landingMediaRepo: landingMediaRepo,
		logger:           slog.Default(),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *assetService) UploadObject(
	ctx context.Context,
	scope coretenant.Scope,
	params UploadObjectParams,
	content io.Reader,
) (domain.AssetObject, error) {
	if params.SizeBytes > MaxObjectSizeBytes {
		return domain.AssetObject{}, ErrFileTooLarge
	}
	mimeLower := strings.ToLower(params.MimeType)
	if !allowedObjectMimeTypes[mimeLower] {
		return domain.AssetObject{}, ErrInvalidMimeType
	}
	if s.objectStorage == nil {
		return domain.AssetObject{}, ErrStorageRequired
	}
	if content == nil {
		return domain.AssetObject{}, errors.New("object content is required")
	}

	if err := s.checkQuota(ctx, scope, params.SizeBytes); err != nil {
		return domain.AssetObject{}, err
	}

	class := params.Class
	if !class.IsValid() {
		class = domain.ObjectClassPrivate
	}

	key, err := storage.NewRandomObjectKey(storage.ObjectKeyInput{
		Scope:     scope,
		Class:     storage.ObjectClass(class),
		Namespace: assetStorageNamespace,
		Filename:  params.Filename,
		Extension: path.Ext(params.Filename),
	})
	if err != nil {
		return domain.AssetObject{}, err
	}

	limited := io.LimitReader(content, MaxObjectSizeBytes+1)
	if err := s.objectStorage.Put(ctx, key, limited, mimeLower, params.SizeBytes); err != nil {
		return domain.AssetObject{}, err
	}

	object, err := s.assetRepo.Create(ctx, scope, repository.CreateAssetObjectParams{
		StorageKey: key,
		Filename:   params.Filename,
		MimeType:   mimeLower,
		SizeBytes:  params.SizeBytes,
		Class:      class,
		Label:      params.Label,
		CreatedBy:  params.CreatedBy,
	})
	if err != nil {
		// Row gagal dibuat — bersihkan file agar tidak yatim.
		_ = s.objectStorage.Delete(ctx, key)
		return domain.AssetObject{}, err
	}
	return object, nil
}

func (s *assetService) ListObjects(ctx context.Context, scope coretenant.Scope) ([]domain.AssetObject, error) {
	return s.assetRepo.List(ctx, scope)
}

func (s *assetService) DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error {
	object, err := s.assetRepo.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if err := s.assetRepo.Delete(ctx, scope, id); err != nil {
		return err
	}
	if s.objectStorage != nil && object.StorageKey != "" {
		// Best-effort: row sudah terhapus, file yatim tidak fatal.
		_ = s.objectStorage.Delete(ctx, object.StorageKey)
	}
	return nil
}

func (s *assetService) DownloadURL(ctx context.Context, scope coretenant.Scope, id string) (DownloadResult, error) {
	object, err := s.assetRepo.Get(ctx, scope, id)
	if err != nil {
		return DownloadResult{}, err
	}
	if s.objectStorage == nil {
		return DownloadResult{}, ErrStorageRequired
	}
	presigner, ok := s.objectStorage.(storage.PresignedStorage)
	if !ok {
		return DownloadResult{}, storage.ErrPresignUnsupported
	}
	presigned, err := presigner.PresignGet(ctx, object.StorageKey)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		ObjectKey:   object.StorageKey,
		DownloadURL: presigned.URL,
		Method:      presigned.Method,
		ExpiresAt:   presigned.ExpiresAt,
	}, nil
}

func (s *assetService) GetUsage(ctx context.Context, scope coretenant.Scope) (StorageUsage, error) {
	used, err := s.usedBytes(ctx, scope)
	if err != nil {
		return StorageUsage{}, err
	}
	limit := s.effectiveLimitBytes(ctx, scope.OrganizationID())
	return StorageUsage{UsedBytes: used, LimitBytes: limit}, nil
}

// usedBytes menjumlahkan pemakaian lintas 2 tabel: asset_objects (modul ini)
// dan landing_media_assets (modul landing) — disengaja, storage tenant
// dihitung gabungan terlepas dari lewat modul mana file itu diunggah.
func (s *assetService) usedBytes(ctx context.Context, scope coretenant.Scope) (int64, error) {
	assetTotal, err := s.assetRepo.SumSizeBytes(ctx, scope)
	if err != nil {
		return 0, err
	}
	if s.landingMediaRepo == nil {
		return assetTotal, nil
	}
	landingTotal, err := s.landingMediaRepo.SumSizeBytes(ctx, scope)
	if err != nil {
		return 0, err
	}
	return assetTotal + landingTotal, nil
}

func (s *assetService) checkQuota(ctx context.Context, scope coretenant.Scope, additionalBytes int64) error {
	limit := s.effectiveLimitBytes(ctx, scope.OrganizationID())
	if limit == nil {
		return nil
	}
	used, err := s.usedBytes(ctx, scope)
	if err != nil {
		return err
	}
	if used+additionalBytes > *limit {
		return ErrQuotaExceeded
	}
	return nil
}

// effectiveLimitBytes membaca limit storage.max_bytes yang berlaku. Tidak
// ada entitlement row / error pencarian dianggap "belum diset" (unlimited),
// bukan ditolak keras — lihat komentar EntitlementFinder di asset_contract.go.
func (s *assetService) effectiveLimitBytes(ctx context.Context, organizationID string) *int64 {
	if s.entitlementFinder == nil {
		return nil
	}
	entitlement, err := s.entitlementFinder.FindEffective(ctx, organizationID, featureStorageMaxBytes, time.Now())
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("asset: gagal baca entitlement storage.max_bytes, dianggap unlimited",
				"organization_id", organizationID, "error", err)
		}
		return nil
	}
	limit, err := extractLimit(entitlement.Limits, storageLimitKeyBytes)
	if err != nil {
		s.logger.Warn("asset: limit storage.max_bytes tidak valid, dianggap unlimited",
			"organization_id", organizationID, "error", err)
		return nil
	}
	return limit
}

// ScopeForOrganization membangun coretenant.Scope langsung dari organization
// ID mentah — dipakai endpoint platform (super admin) yang perlu baca usage
// storage tenant LAIN, bukan tenant milik pemanggil sendiri. Field
// slug/type/status di bawah cuma placeholder (pola sama seperti
// delivery_service.go's processDelivery) — coretenant.Scope yang jadi
// keluarannya cuma pernah menyimpan organizationID + dataPlacement, jadi
// nilai placeholder ini tidak pernah dipakai lagi setelahnya.
func ScopeForOrganization(organizationID string) (coretenant.Scope, error) {
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     organizationID,
		OrganizationSlug:   "platform-lookup",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceInternal,
		DataPlacement:      coretenant.DataPlacementShared,
		IsPlatformOperator: true,
	})
	if err != nil {
		return coretenant.Scope{}, err
	}
	return coretenant.NewScope(tc)
}

// extractLimit meniru guardLimit di subscription_guard_service.go (unexported
// di sana, jadi disalin di sini) — mengambil nilai numerik dari limits map.
func extractLimit(limits map[string]any, key string) (*int64, error) {
	if limits == nil {
		return nil, nil
	}
	value, exists := limits[key]
	if !exists || value == nil {
		return nil, nil
	}
	var limit int64
	switch typed := value.(type) {
	case int:
		limit = int64(typed)
	case int32:
		limit = int64(typed)
	case int64:
		limit = typed
	case float64:
		if math.Trunc(typed) != typed || typed > math.MaxInt64 {
			return nil, errors.New("entitlement limit is invalid")
		}
		limit = int64(typed)
	default:
		return nil, errors.New("entitlement limit has unexpected type")
	}
	return &limit, nil
}
