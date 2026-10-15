package service

import (
	"context"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogmodule "zyad.cloud/internal/modules/catalog"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
)

type categoryService struct{ repo repository.CategoryRepository }

func NewCategoryService(repo repository.CategoryRepository) CategoryService {
	return &categoryService{repo: repo}
}

func (s *categoryService) Create(ctx context.Context, scope coretenant.Scope, name string, position int, userID string) (domain.Category, error) {
	if !validText(name, 100, true) {
		return domain.Category{}, ErrInvalidCategory
	}
	return s.repo.Create(ctx, scope, strings.TrimSpace(name), position, userID)
}

func (s *categoryService) List(ctx context.Context, scope coretenant.Scope) ([]domain.Category, error) {
	return s.repo.List(ctx, scope)
}

func (s *categoryService) Update(ctx context.Context, scope coretenant.Scope, id string, name *string, position *int, userID string) (domain.Category, error) {
	if name != nil && !validText(*name, 100, true) {
		return domain.Category{}, ErrInvalidCategory
	}
	c, err := s.repo.Update(ctx, scope, id, name, position, userID)
	return c, catalogmodule.MapNotFound(err, "CATEGORY_NOT_FOUND", "category not found or already deleted")
}

func (s *categoryService) Delete(ctx context.Context, scope coretenant.Scope, id, userID string) error {
	return catalogmodule.MapNotFound(s.repo.Delete(ctx, scope, id, userID), "CATEGORY_NOT_FOUND", "category not found or already deleted")
}
