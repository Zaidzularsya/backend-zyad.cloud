package organization

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

// Kode error guard entitlement. Nilai string dipertahankan dari modul
// subscription lama karena sudah menjadi kontrak API (FE membaca kode ini).
const (
	ErrCodeFeatureNotEnabled = "FEATURE_NOT_ENABLED"
	ErrCodeQuotaExceeded     = "QUOTA_EXCEEDED"
)

func FeatureNotEnabledError() error {
	return coreerrors.New(ErrCodeFeatureNotEnabled, "billing feature is not enabled", http.StatusForbidden)
}

func QuotaExceededError() error {
	return coreerrors.New(ErrCodeQuotaExceeded, "billing quota has been exceeded", http.StatusConflict)
}
