package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
)

type fakeProducts struct {
	repository.ProductRepository
	created []repository.CreateProductParams
	updated []repository.UpdateProductParams
}

func (f *fakeProducts) Create(_ context.Context, _ coretenant.Scope, p repository.CreateProductParams) (domain.Product, error) {
	f.created = append(f.created, p)
	return domain.Product{ID: "p1", Name: p.Name}, nil
}

func (f *fakeProducts) Update(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateProductParams) (domain.Product, error) {
	f.updated = append(f.updated, p)
	return domain.Product{ID: id}, nil
}

// Service tidak memvalidasi scope (tugas repository); fake mengabaikannya.
func scope(t *testing.T) coretenant.Scope {
	t.Helper()
	return coretenant.Scope{}
}

func TestValidators(t *testing.T) {
	for _, ok := range []string{"0", "350000", "350000.5", "350000.50"} {
		if !ValidMoney(ok) {
			t.Errorf("ValidMoney(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "-1", "1.234", "1,5", "Rp1"} {
		if ValidMoney(bad) {
			t.Errorf("ValidMoney(%q) = true", bad)
		}
	}
	for _, ok := range []string{"0", "11", "12.5", "100"} {
		if !ValidPercent(ok) {
			t.Errorf("ValidPercent(%q) = false", ok)
		}
	}
	for _, bad := range []string{"100.01", "-1", "abc", ""} {
		if ValidPercent(bad) {
			t.Errorf("ValidPercent(%q) = true", bad)
		}
	}
}

func TestCreateProductValidatesAndDefaults(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo)
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: " ", Unit: "pcs", BasePrice: "1"}, "u1"); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("blank name err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: "A", Unit: "pcs", BasePrice: "1", TaxPercent: "101"}, "u1"); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("tax err = %v", err)
	}
	if _, err := svc.Create(context.Background(), scope(t), ProductInput{Name: " Internet ", Unit: "", BasePrice: "", IsActive: true}, "u1"); err != nil {
		t.Fatal(err)
	}
	got := repo.created[0]
	if got.Name != "Internet" || got.Unit != "pcs" || got.BasePrice != "0" || got.TaxPercent != "0" || got.CreatedBy != "u1" {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestUpdateProductValidatesOnlyProvidedFields(t *testing.T) {
	repo := &fakeProducts{}
	svc := NewProductService(repo)
	bad := "abc"
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{BasePrice: &bad}); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("err = %v", err)
	}
	active := false
	if _, err := svc.Update(context.Background(), scope(t), "p1", repository.UpdateProductParams{IsActive: &active}); err != nil {
		t.Fatal(err)
	}
}
