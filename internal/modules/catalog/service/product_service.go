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
	"zyad.cloud/internal/shared/pricing"
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

type productService struct {
	repo     repository.ProductRepository
	features FeatureRegistry
}

func NewProductService(repo repository.ProductRepository, features FeatureRegistry) ProductService {
	return &productService{repo: repo, features: features}
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
	attrs, err := pricing.Normalize(in.Pricing)
	if err != nil || !validText(in.Name, 200, true) || !validText(in.Unit, 30, true) || !validText(in.SKU, 64, false) ||
		!ValidMoney(in.BasePrice) || !ValidPercent(in.TaxPercent) {
		return domain.Product{}, ErrInvalidProduct
	}
	if !in.Platform && (in.Listing != (repository.ListingParams{}) || len(in.Features) > 0) {
		return domain.Product{}, ErrPlatformOnly
	}
	if in.Platform {
		if err := validateListing(in.Listing, in.CategoryID); err != nil {
			return domain.Product{}, err
		}
		if err := s.validateFeatures(ctx, in.Features); err != nil {
			return domain.Product{}, err
		}
	}
	in.Listing.ListingCode = strings.TrimSpace(in.Listing.ListingCode)
	p, err := s.repo.Create(ctx, scope, repository.CreateProductParams{
		CategoryID: in.CategoryID, SKU: in.SKU, Name: in.Name, Description: in.Description, Unit: in.Unit,
		BasePrice: in.BasePrice, TaxPercent: in.TaxPercent, Pricing: attrs, IsActive: in.IsActive, CreatedBy: userID,
		Listing: in.Listing, Features: in.Features,
	})
	return s.withLabels(ctx, p, err)
}

// withLabels melewatkan error repository apa adanya, dan memperkaya label fitur bila sukses.
func (s *productService) withLabels(ctx context.Context, p domain.Product, err error) (domain.Product, error) {
	if err != nil {
		return p, err
	}
	list := []domain.Product{p}
	if err := fillFeatureLabels(ctx, s.features, list); err != nil {
		return p, err
	}
	return list[0], nil
}

func (s *productService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Product, error) {
	p, err := s.repo.FindByID(ctx, scope, id)
	err = catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
	return s.withLabels(ctx, p, err)
}

func (s *productService) FindBySKU(ctx context.Context, scope coretenant.Scope, sku string) (domain.Product, error) {
	p, err := s.repo.FindBySKU(ctx, scope, strings.TrimSpace(sku))
	err = catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
	return s.withLabels(ctx, p, err)
}

func (s *productService) List(ctx context.Context, scope coretenant.Scope, f repository.ProductListFilter) ([]domain.Product, int64, error) {
	list, total, err := s.repo.List(ctx, scope, f)
	if err != nil {
		return list, total, err
	}
	return list, total, fillFeatureLabels(ctx, s.features, list)
}

func (s *productService) FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]domain.Product, error) {
	m, err := s.repo.FindByIDs(ctx, scope, ids)
	if err != nil {
		return m, err
	}
	list := make([]domain.Product, 0, len(m))
	for _, p := range m {
		list = append(list, p)
	}
	if err := fillFeatureLabels(ctx, s.features, list); err != nil {
		return m, err
	}
	for _, p := range list {
		m[p.ID] = p
	}
	return m, nil
}

func (s *productService) ListFeatures(ctx context.Context) ([]domain.FeatureDef, error) {
	return s.features.ListActive(ctx)
}

// validatePlatformUpdate memeriksa blok listing/fitur pada update. Kategori efektif
// (yang tersimpan, atau yang baru dikirim) menentukan sah-tidaknya produk publik.
func (s *productService) validatePlatformUpdate(ctx context.Context, scope coretenant.Scope, id string, in repository.UpdateProductParams) error {
	if in.Features != nil {
		if err := s.validateFeatures(ctx, *in.Features); err != nil {
			return err
		}
	}
	if in.Listing == nil && in.CategoryID == nil {
		return nil
	}
	cur, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
	}
	listing := repository.ListingParams{IsPublic: cur.IsPublic, ListingCode: cur.ListingCode, ListingOrder: cur.ListingOrder}
	if in.Listing != nil {
		listing = *in.Listing
	}
	category := ""
	if cur.CategoryID != nil {
		category = *cur.CategoryID
	}
	if in.CategoryID != nil {
		category = *in.CategoryID
	}
	return validateListing(listing, category)
}

func (s *productService) Update(ctx context.Context, scope coretenant.Scope, id string, in repository.UpdateProductParams, platform bool) (domain.Product, error) {
	if !platform && (in.Listing != nil || in.Features != nil) {
		return domain.Product{}, ErrPlatformOnly
	}
	if (in.Name != nil && !validText(*in.Name, 200, true)) || (in.Unit != nil && !validText(*in.Unit, 30, true)) ||
		(in.SKU != nil && !validText(*in.SKU, 64, false)) ||
		(in.BasePrice != nil && !ValidMoney(*in.BasePrice)) || (in.TaxPercent != nil && !ValidPercent(*in.TaxPercent)) {
		return domain.Product{}, ErrInvalidProduct
	}
	if in.Pricing != nil {
		attrs, err := pricing.Normalize(*in.Pricing)
		if err != nil {
			return domain.Product{}, ErrInvalidProduct
		}
		in.Pricing = &attrs
	}
	if platform {
		if err := s.validatePlatformUpdate(ctx, scope, id, in); err != nil {
			return domain.Product{}, err
		}
	}
	if in.Listing != nil {
		l := *in.Listing
		l.ListingCode = strings.TrimSpace(l.ListingCode)
		in.Listing = &l
	}
	p, err := s.repo.Update(ctx, scope, id, in)
	err = catalogmodule.MapNotFound(err, "PRODUCT_NOT_FOUND", "product not found or already deleted")
	return s.withLabels(ctx, p, err)
}

func (s *productService) Delete(ctx context.Context, scope coretenant.Scope, id, userID string) error {
	return catalogmodule.MapNotFound(s.repo.Delete(ctx, scope, id, userID), "PRODUCT_NOT_FOUND", "product not found or already deleted")
}
