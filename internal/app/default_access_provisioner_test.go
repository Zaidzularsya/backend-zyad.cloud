package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

type daScopes struct{}

func (daScopes) PlatformScope(context.Context) (coretenant.Scope, error) {
	return coretenant.Scope{}, nil
}

type daProducts struct {
	product *catalogdomain.Product
	asked   string
}

func (p *daProducts) FindBySKU(_ context.Context, _ coretenant.Scope, sku string) (catalogdomain.Product, error) {
	p.asked = sku
	if p.product == nil {
		return catalogdomain.Product{}, coreerrors.New("PRODUCT_NOT_FOUND", "product not found", 404)
	}
	return *p.product, nil
}

type daFeatures struct{}

func (daFeatures) FindByKeys(context.Context, []string) (map[string]catalogdomain.FeatureDef, error) {
	return map[string]catalogdomain.FeatureDef{
		"user.max":    {Key: "user.max", ValueType: "integer"},
		"crm.enabled": {Key: "crm.enabled", ValueType: "boolean"},
	}, nil
}

type daWriter struct {
	org    string
	grants []organizationservice.FeatureGrant
}

func (w *daWriter) GrantDefault(_ context.Context, org string, grants []organizationservice.FeatureGrant, _ string) error {
	w.org, w.grants = org, grants
	return nil
}

func TestProvisionDefaultAccessGrantsProductFeatures(t *testing.T) {
	products := &daProducts{product: &catalogdomain.Product{Features: []catalogdomain.ProductFeature{
		{FeatureKey: "user.max", Value: json.RawMessage(`1`)},
		{FeatureKey: "crm.enabled", Value: json.RawMessage(`true`)},
		{FeatureKey: "legacy.gone", Value: json.RawMessage(`1`)}, // tidak ada di registry → dilewati
	}}}
	writer := &daWriter{}
	p := defaultAccessProvisioner{scopes: daScopes{}, products: products, features: daFeatures{}, writer: writer, sku: "FREE"}
	if err := p.ProvisionDefaultAccess(context.Background(), "org-1"); err != nil {
		t.Fatal(err)
	}
	if products.asked != "FREE" || writer.org != "org-1" || len(writer.grants) != 2 ||
		writer.grants[0].Key != "user.max" || writer.grants[0].ValueType != "integer" || writer.grants[1].ValueType != "boolean" {
		t.Fatalf("asked=%q org=%q grants=%+v", products.asked, writer.org, writer.grants)
	}
}

func TestProvisionDefaultAccessMissingProductThenRecovers(t *testing.T) {
	products := &daProducts{}
	writer := &daWriter{}
	p := defaultAccessProvisioner{scopes: daScopes{}, products: products, features: daFeatures{}, writer: writer, sku: "FREE"}
	if err := p.ProvisionDefaultAccess(context.Background(), "org-1"); !errors.Is(err, ErrDefaultProductNotFound) {
		t.Fatalf("err = %v, want DEFAULT_PRODUCT_NOT_FOUND", err)
	}
	if writer.org != "" {
		t.Fatal("nothing must be written when the default product is missing")
	}
	products.product = &catalogdomain.Product{Features: []catalogdomain.ProductFeature{{FeatureKey: "user.max", Value: json.RawMessage(`1`)}}}
	if err := p.ProvisionDefaultAccess(context.Background(), "org-1"); err != nil || len(writer.grants) != 1 {
		t.Fatalf("second call err=%v grants=%+v", err, writer.grants)
	}
}
