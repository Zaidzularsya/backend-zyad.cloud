package product

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const (
	ErrCodePlanNotFound      = "PLAN_NOT_FOUND"
	ErrCodePlanPriceNotFound = "PLAN_PRICE_NOT_FOUND"
	ErrCodeFeatureNotFound   = "FEATURE_NOT_FOUND"
)

func PlanNotFoundError() error {
	return coreerrors.New(ErrCodePlanNotFound, "billing plan not found", http.StatusNotFound)
}

func PlanPriceNotFoundError() error {
	return coreerrors.New(ErrCodePlanPriceNotFound, "billing plan price not found", http.StatusNotFound)
}

func FeatureNotFoundError() error {
	return coreerrors.New(ErrCodeFeatureNotFound, "billing feature not found", http.StatusNotFound)
}
