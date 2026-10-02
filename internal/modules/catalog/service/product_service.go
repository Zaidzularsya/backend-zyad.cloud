package service

import (
	"context"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogmodule "zyad.cloud/internal/modules/catalog"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
)

var moneyPattern = regexp.MustCompile(`^\d{1,16}(\.\d{1,2})?$`)
var percentPattern = regexp.MustCompile(`^\d{1,3}(\.\d{1,2})?$`)

func ValidMoney(v string) bool { return moneyPattern.MatchString(v) }

func ValidPercent(v string) bool {
	if !percentPattern.MatchString(v) {
		return false
	}
	r, _ := new(big.Rat).SetString(v)
	return r.Cmp(big.NewRat(100, 1)) <= 0
}

type productService struct{ repo repository.ProductRepository }

func NewProductService(repo repository.ProductRepository) ProductService {
	return &productService{repo: repo}
}

func validText(v string, max int, required bool) bool {
	v = strings.TrimSpace(v)
	return (!required || v != "") && utf8.RuneCountInString(v) <= max
}

func (s *productService) Create(ctx context.Context, scope coretenant.Scope, in ProductInput, userID string) (domain.Product, error) {
	in.Name, in.Unit, in.SKU = strings.TrimSpace(in.Name), strings.TrimSpace(in.Unit), strings.TrimSpace(in.SKU)
	if in.Unit == "" {
		in.Unit = "pcs"
	}
	if in.BasePrice == "" {
		in.BasePrice = "0"
	}
	if in.TaxPercent == "" {
		in.TaxPercent = "0"
	}
	if !validText(in.Name, 200, true) || !validText(in.Unit, 30, true) || !validText(in.SKU, 64, false) ||
		!ValidMoney(in.BasePrice) || !ValidPercent(in.TaxPercent) {
		return domain.Product{}, ErrInvalidProduct
	}
	return s.repo.Create(ctx, scope, repository.CreateProductParams{
		CategoryID: in.CategoryID, SKU: in.SKU, Name: in.Name, Description: in.Description, Unit: in.Unit,
		BasePrice: in.BasePrice, TaxPercent: in.TaxPercent, IsActive: in.IsActive, CreatedBy: userID,
	})
}

func (s *productService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Product, error) {
	p, err := s.repo.FindByID(ctx, scope, id)
	return p, catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
}

func (s *productService) List(ctx context.Context, scope coretenant.Scope, f repository.ProductListFilter) ([]domain.Product, int64, error) {
	return s.repo.List(ctx, scope, f)
}

func (s *productService) Update(ctx context.Context, scope coretenant.Scope, id string, in repository.UpdateProductParams) (domain.Product, error) {
	if (in.Name != nil && !validText(*in.Name, 200, true)) || (in.Unit != nil && !validText(*in.Unit, 30, true)) ||
		(in.SKU != nil && !validText(*in.SKU, 64, false)) ||
		(in.BasePrice != nil && !ValidMoney(*in.BasePrice)) || (in.TaxPercent != nil && !ValidPercent(*in.TaxPercent)) {
		return domain.Product{}, ErrInvalidProduct
	}
	p, err := s.repo.Update(ctx, scope, id, in)
	return p, catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
}

func (s *productService) Delete(ctx context.Context, scope coretenant.Scope, id, userID string) error {
	return catalogmodule.MapNotFound(s.repo.Delete(ctx, scope, id, userID), "PRODUCT_NOT_FOUND", "product not found or already deleted")
}
