package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/modules/catalog/service"
)

func failCatalog(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidProduct), errors.Is(err, service.ErrInvalidCategory):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
	case errors.Is(err, repository.ErrSKUExists):
		corehttp.Fail(c, coreerrors.New("PRODUCT_SKU_EXISTS", "SKU sudah dipakai produk lain", http.StatusConflict))
	case errors.Is(err, repository.ErrCategoryNameExists):
		corehttp.Fail(c, coreerrors.New("CATEGORY_NAME_EXISTS", "Nama kategori sudah ada", http.StatusConflict))
	default:
		corehttp.Fail(c, err)
	}
}

func validationError(c *gin.Context, err error) {
	corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
}

func missingScope(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
}
