package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type activationReader struct {
	repo repository.ActivationRepository
}

func NewActivationReader(repo repository.ActivationRepository) ActivationReader {
	return &activationReader{repo: repo}
}

func (s *activationReader) OpenActivationsByPages(ctx context.Context, scope tenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}
	return s.repo.ListOpenByPages(ctx, scope, pageIDs)
}
