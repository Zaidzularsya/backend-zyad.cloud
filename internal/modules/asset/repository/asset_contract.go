package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
)

type CreateAssetObjectParams struct {
	StorageKey string
	Filename   string
	MimeType   string
	SizeBytes  int64
	Class      domain.ObjectClass
	Label      string
	CreatedBy  string
}

type AssetRepository interface {
	Create(context.Context, coretenant.Scope, CreateAssetObjectParams) (domain.AssetObject, error)
	Get(context.Context, coretenant.Scope, string) (domain.AssetObject, error)
	List(context.Context, coretenant.Scope) ([]domain.AssetObject, error)
	Delete(context.Context, coretenant.Scope, string) error
	// SumSizeBytes menjumlahkan size_bytes semua object aktif (belum dihapus)
	// milik organization — dipakai untuk hitung total pemakaian storage tenant.
	SumSizeBytes(context.Context, coretenant.Scope) (int64, error)
}
