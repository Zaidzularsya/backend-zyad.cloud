package service

import (
	"context"
	"fmt"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

type defaultDomainService struct {
	repo     repository.DomainRepository
	pageRepo repository.PageRepository
	db       *database.Pool
	features DomainFeatureGate
}

type DomainFeatureGate interface {
	RequireFeature(
		context.Context,
		string,
		string,
	) (organizationmodel.Entitlement, error)
}

type DomainServiceOption func(*defaultDomainService)

func WithLandingDomainFeatureGate(features DomainFeatureGate) DomainServiceOption {
	return func(service *defaultDomainService) {
		service.features = features
	}
}

func NewDomainService(
	repo repository.DomainRepository,
	pageRepo repository.PageRepository,
	db *database.Pool,
	options ...DomainServiceOption,
) DomainService {
	service := &defaultDomainService{
		repo:     repo,
		pageRepo: pageRepo,
		db:       db,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *defaultDomainService) BindDomain(ctx context.Context, scope coretenant.Scope, params BindDomainParams) (domain.DomainBinding, error) {
	// First check if the domain exists and is verified in the organization
	availableDomains, err := s.ListAvailableDomains(ctx, scope)
	if err != nil {
		return domain.DomainBinding{}, fmt.Errorf("check available domains: %w", err)
	}

	var matchedDomain domain.AvailableDomain
	domainVerified := false
	for _, d := range availableDomains {
		if d.ID == params.OrganizationDomainID {
			matchedDomain = d
			domainVerified = true
			break
		}
	}

	if !domainVerified {
		return domain.DomainBinding{}, coreerrors.New(
			"DOMAIN_NOT_AVAILABLE",
			"domain is not available or not verified",
			http.StatusNotFound,
		)
	}

	// Custom domains require an entitlement (billing feature); platform subdomains
	// don't consume that entitlement and must always be bindable.
	if matchedDomain.Type == string(organizationmodel.DomainTypeCustom) {
		if err := s.requireCustomDomainFeature(ctx, scope.OrganizationID()); err != nil {
			return domain.DomainBinding{}, err
		}
	}

	// Verify the page exists
	_, err = s.pageRepo.FindByID(ctx, scope, params.LandingPageID)
	if err != nil {
		return domain.DomainBinding{}, fmt.Errorf("verify page exists: %w", err)
	}

	// Check if binding already exists
	bindings, err := s.repo.ListBindings(ctx, scope, params.LandingPageID)
	if err != nil {
		return domain.DomainBinding{}, fmt.Errorf("list bindings: %w", err)
	}

	for _, b := range bindings {
		if b.OrganizationDomainID == params.OrganizationDomainID {
			return domain.DomainBinding{}, coreerrors.New(
				"DOMAIN_ALREADY_BOUND",
				"domain is already bound to this page",
				http.StatusConflict,
			)
		}
	}

	// If this is the first binding, we might want to make it primary automatically
	if len(bindings) == 0 {
		params.IsPrimary = true
	}

	repoParams := repository.BindDomainParams{
		OrganizationDomainID: params.OrganizationDomainID,
		LandingPageID:        params.LandingPageID,
		IsPrimary:            params.IsPrimary,
	}

	return s.repo.BindDomain(ctx, scope, repoParams)
}

func (s *defaultDomainService) UnbindDomain(ctx context.Context, scope coretenant.Scope, bindingID string) error {
	return s.repo.UnbindDomain(ctx, scope, bindingID)
}

func (s *defaultDomainService) SetPrimaryBinding(ctx context.Context, scope coretenant.Scope, pageID string, bindingID string) error {
	return s.repo.SetPrimaryBinding(ctx, scope, pageID, bindingID)
}

func (s *defaultDomainService) ListBindings(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.DomainBinding, error) {
	return s.repo.ListBindings(ctx, scope, pageID)
}

func (s *defaultDomainService) ListAllBindings(ctx context.Context, scope coretenant.Scope) ([]domain.DomainBinding, error) {
	return s.repo.ListAllBindings(ctx, scope)
}

func (s *defaultDomainService) ListAvailableDomains(ctx context.Context, scope coretenant.Scope) ([]domain.AvailableDomain, error) {
	return s.repo.ListAvailableDomains(ctx, scope)
}

func (s *defaultDomainService) requireCustomDomainFeature(
	ctx context.Context,
	organizationID string,
) error {
	if s == nil || s.features == nil {
		return nil
	}
	_, err := s.features.RequireFeature(ctx, organizationID, domain.FeatureLandingCustomDomain)
	return err
}
