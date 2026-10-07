package product

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const ErrCodeFeatureNotFound = "FEATURE_NOT_FOUND"

func FeatureNotFoundError() error {
	return coreerrors.New(ErrCodeFeatureNotFound, "billing feature not found", http.StatusNotFound)
}
