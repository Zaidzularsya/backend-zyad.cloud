package service

import (
	"context"
	"io"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type UploadObjectParams struct {
	Filename  string
	MimeType  string
	SizeBytes int64
	Class     domain.ObjectClass
	Label     string
	CreatedBy string
}

type DownloadResult struct {
	ObjectKey   string    `json:"object_key"`
	DownloadURL string    `json:"download_url"`
	Method      string    `json:"method"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type StorageUsage struct {
	UsedBytes  int64  `json:"used_bytes"`
	LimitBytes *int64 `json:"limit_bytes,omitempty"`
}

type AssetService interface {
	UploadObject(ctx context.Context, scope coretenant.Scope, params UploadObjectParams, content io.Reader) (domain.AssetObject, error)
	ListObjects(ctx context.Context, scope coretenant.Scope) ([]domain.AssetObject, error)
	DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error
	DownloadURL(ctx context.Context, scope coretenant.Scope, id string) (DownloadResult, error)
	// GetUsage menjumlahkan pemakaian storage tenant gabungan (asset_objects +
	// landing_media_assets) dan, kalau tersedia, limit kuotanya.
	GetUsage(ctx context.Context, scope coretenant.Scope) (StorageUsage, error)
}

// LandingMediaSumRepository adalah subset kecil dari
// landing/repository.MediaRepository — dipakai supaya modul asset tidak perlu
// import seluruh repository landing, cukup kemampuan hitung total ukurannya.
type LandingMediaSumRepository interface {
	SumSizeBytes(context.Context, coretenant.Scope) (int64, error)
}

// EntitlementFinder adalah subset kecil dari
// organizationrepo.EntitlementRepository — dipakai baik untuk cek kuota
// sebelum upload maupun untuk endpoint platform yang menampilkan limit ke
// super admin.
//
// Storage TIDAK memakai pola AssetQuotaGuard/SubscriptionGuardService penuh
// seperti landing.max_pages (yang mewajibkan subscription aktif + entitlement
// row, gagal keras kalau tidak ada) — feature key storage.max_bytes ini baru,
// belum ada di plan manapun, jadi kalau dipaksa lewat RequireQuotaValue,
// SEMUA tenant langsung tidak bisa upload sama sekali sejak hari pertama
// deploy. Sebagai gantinya: tidak ada entitlement row = dianggap unlimited
// (sama seperti perilaku hari ini, yang memang tidak ada kuota storage sama
// sekali). Super admin baru membatasi kalau memang men-set limit lewat
// PATCH /platform/organizations/:id/entitlements.
type EntitlementFinder interface {
	FindEffective(ctx context.Context, organizationID string, featureKey string, at time.Time) (organizationmodel.Entitlement, error)
}
