package service

import (
	"context"
	"errors"
	"math/big"
	"sort"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/dto"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

// ErrPlatformNotFound: org platform belum ada (instalasi baru); listing publik kosong.
var ErrPlatformNotFound = errors.New("platform organization not found")

// PlatformScopeResolver menyediakan scope org platform untuk endpoint publik tanpa auth.
type PlatformScopeResolver interface {
	PlatformScope(ctx context.Context) (coretenant.Scope, error)
}

type PublicListingService struct {
	products repository.ProductRepository
	features FeatureRegistry
	scopes   PlatformScopeResolver
}

func NewPublicListingService(products repository.ProductRepository, features FeatureRegistry, scopes PlatformScopeResolver) *PublicListingService {
	return &PublicListingService{products: products, features: features, scopes: scopes}
}

// Listings menyusun produk publik platform: kategori → kartu (listing_code) → varian (frekuensi).
func (s *PublicListingService) Listings(ctx context.Context) (dto.PublicCatalogResponse, error) {
	empty := dto.PublicCatalogResponse{Categories: []dto.PublicCategory{}}
	scope, err := s.scopes.PlatformScope(ctx)
	if errors.Is(err, ErrPlatformNotFound) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	products, err := s.products.ListPublic(ctx, scope)
	if err != nil {
		return empty, err
	}
	if err := fillFeatureLabels(ctx, s.features, products); err != nil {
		return empty, err
	}

	sort.SliceStable(products, func(i, j int) bool {
		a, b := products[i], products[j]
		if ra, rb := domain.FrequencyRank(a.Pricing.Frequency), domain.FrequencyRank(b.Pricing.Frequency); ra != rb {
			return ra < rb
		}
		return a.SKU < b.SKU
	})

	type cardKey struct{ category, code string }
	categories := map[string]*dto.PublicCategory{}
	cards := map[cardKey]*dto.PublicListing{}
	var catOrder []string
	for _, p := range products {
		if p.CategoryID == nil {
			continue
		}
		cat, ok := categories[*p.CategoryID]
		if !ok {
			cat = &dto.PublicCategory{ID: *p.CategoryID, Name: p.CategoryName, Position: p.CategoryPosition, Listings: []dto.PublicListing{}}
			categories[*p.CategoryID] = cat
			catOrder = append(catOrder, *p.CategoryID)
		}
		key := cardKey{*p.CategoryID, p.ListingCode}
		card, ok := cards[key]
		if !ok {
			// Varian pertama = frekuensi terkecil; ia yang menentukan wajah kartu.
			card = &dto.PublicListing{Code: p.ListingCode, Name: p.Name, Description: p.Description, Order: p.ListingOrder,
				Variants: []dto.PublicVariant{}, Benefits: visibleBenefits(p)}
			cards[key] = card
		}
		priced, err := priceWithTax(p)
		if err != nil {
			return empty, err
		}
		card.Variants = append(card.Variants, dto.PublicVariant{
			ProductID: p.ID, SKU: p.SKU, ChargeType: string(p.Pricing.ChargeType), BillingFrequency: string(p.Pricing.Frequency),
			PaymentTiming: string(p.Pricing.PaymentTiming), Currency: p.Currency, BasePrice: p.BasePrice, TaxPercent: p.TaxPercent,
			PriceWithTax: priced, CheckoutEnabled: checkoutEnabled(p),
		})
	}

	out := dto.PublicCatalogResponse{Categories: make([]dto.PublicCategory, 0, len(catOrder))}
	for _, id := range catOrder {
		cat := categories[id]
		cat.Listings = cat.Listings[:0]
		for key, card := range cards {
			if key.category == id {
				cat.Listings = append(cat.Listings, *card)
			}
		}
		sort.Slice(cat.Listings, func(i, j int) bool {
			a, b := cat.Listings[i], cat.Listings[j]
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			return a.Code < b.Code
		})
		out.Categories = append(out.Categories, *cat)
	}
	sort.SliceStable(out.Categories, func(i, j int) bool {
		a, b := out.Categories[i], out.Categories[j]
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.Name < b.Name
	})
	return out, nil
}

func visibleBenefits(p domain.Product) []dto.PublicBenefit {
	out := []dto.PublicBenefit{}
	for _, f := range p.Features {
		if f.Label != "" {
			out = append(out, dto.PublicBenefit{Label: f.Label})
		}
	}
	return out
}

// priceWithTax memakai kalkulator quote (big.Rat) agar angka publik sama dengan penawaran.
func priceWithTax(p domain.Product) (string, error) {
	_, lines, err := pricing.PriceLines([]pricing.LineInput{{
		Description: p.Name, Quantity: "1", UnitPrice: p.BasePrice, TaxPercent: p.TaxPercent, Attrs: p.Pricing,
	}})
	if err != nil {
		return "", err
	}
	net, ok1 := new(big.Rat).SetString(lines[0].LineTotal)
	tax, ok2 := new(big.Rat).SetString(lines[0].TaxAmount)
	if !ok1 || !ok2 {
		return "", pricing.ErrInvalidLine
	}
	return net.Add(net, tax).FloatString(2), nil
}

// checkoutEnabled: hanya langganan prabayar berharga dan berfitur yang bisa dibeli mandiri.
func checkoutEnabled(p domain.Product) bool {
	price, ok := new(big.Rat).SetString(p.BasePrice)
	return ok && price.Sign() > 0 && p.Pricing.ChargeType == pricing.Recurring &&
		p.Pricing.PaymentTiming == pricing.Prepaid && len(p.Features) > 0
}
