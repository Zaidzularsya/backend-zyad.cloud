package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/repository"
)

type AuthenticatedResolverStore interface {
	FindSessionSnapshot(
		context.Context,
		string,
		string,
	) (repository.SessionOrganizationSnapshot, error)
	FindActiveMembershipOrganization(
		context.Context,
		string,
		string,
	) (repository.MembershipOrganization, error)
	FindSessionMembershipOrganization(
		context.Context,
		string,
		repository.SessionOrganizationSnapshot,
	) (repository.MembershipOrganization, error)
	ListActiveMembershipOrganizations(
		context.Context,
		string,
		int,
	) ([]repository.MembershipOrganization, error)
}

type AuthenticatedResolver struct {
	store AuthenticatedResolverStore
}

func NewAuthenticatedResolver(store AuthenticatedResolverStore) *AuthenticatedResolver {
	return &AuthenticatedResolver{store: store}
}

func (r *AuthenticatedResolver) ResolveAuthenticatedOrganization(
	ctx context.Context,
	userID string,
	sessionID string,
	organizationSelector string,
	requestHost string,
) (coretenant.Context, bool, error) {
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	organizationSelector = strings.TrimSpace(organizationSelector)
	if !validUUID(userID) || !validUUID(sessionID) {
		return coretenant.Context{}, false, coreerrors.New(
			"UNAUTHORIZED",
			"authenticated session is invalid",
			http.StatusUnauthorized,
		)
	}
	if organizationSelector != "" && !validUUID(organizationSelector) {
		return coretenant.Context{}, false, invalidOrganizationSelectorError()
	}
	if r.store == nil {
		return coretenant.Context{}, false, coreerrors.New(
			"TENANT_RESOLVER_STORE_REQUIRED",
			"tenant resolver store is required",
			http.StatusInternalServerError,
		)
	}

	snapshot, err := r.store.FindSessionSnapshot(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coretenant.Context{}, false, coreerrors.New(
				"UNAUTHORIZED",
				"authenticated session is not active",
				http.StatusUnauthorized,
			)
		}
		return coretenant.Context{}, false, resolverInternalError(err)
	}

	if organizationSelector != "" {
		result, err := r.store.FindActiveMembershipOrganization(
			ctx,
			userID,
			organizationSelector,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return coretenant.Context{}, false, coreerrors.New(
					"ORGANIZATION_ACCESS_DENIED",
					"user does not have an active membership in the organization",
					http.StatusForbidden,
				)
			}
			return coretenant.Context{}, false, resolverInternalError(err)
		}
		return verifiedAuthenticatedContext(
			result,
			coretenant.ResolutionSourceHeader,
			requestHost,
		)
	}

	if snapshot.OrganizationID != "" {
		result, err := r.store.FindSessionMembershipOrganization(ctx, userID, snapshot)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return coretenant.Context{}, false, coreerrors.New(
					"ORGANIZATION_CONTEXT_STALE",
					"session organization context is stale",
					http.StatusUnauthorized,
				)
			}
			return coretenant.Context{}, false, resolverInternalError(err)
		}
		return verifiedAuthenticatedContext(
			result,
			coretenant.ResolutionSourceSession,
			requestHost,
		)
	}

	results, err := r.store.ListActiveMembershipOrganizations(ctx, userID, 2)
	if err != nil {
		return coretenant.Context{}, false, resolverInternalError(err)
	}
	if len(results) == 0 {
		return coretenant.Context{}, false, nil
	}
	if len(results) > 1 {
		return coretenant.Context{}, false, nil
	}
	return verifiedAuthenticatedContext(
		results[0],
		coretenant.ResolutionSourceMembership,
		requestHost,
	)
}

func verifiedAuthenticatedContext(
	result repository.MembershipOrganization,
	source coretenant.ResolutionSource,
	requestHost string,
) (coretenant.Context, bool, error) {
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     result.Organization.ID,
		OrganizationSlug:   result.Organization.Slug,
		OrganizationType:   result.Organization.Type,
		OrganizationStatus: result.Organization.Status,
		MembershipID:       result.Membership.ID,
		MembershipStatus:   string(result.Membership.Status),
		MembershipVersion:  result.Membership.Version,
		ResolutionSource:   source,
		DataPlacement:      result.Organization.DataPlacement,
		RequestHost:        strings.TrimSpace(requestHost),
	})
	if err != nil {
		return coretenant.Context{}, false, coreerrors.Wrap(
			"TENANT_CONTEXT_INVALID",
			"resolved tenant context is invalid",
			http.StatusInternalServerError,
			err,
		)
	}
	return tenantContext, true, nil
}

func invalidOrganizationSelectorError() error {
	return coreerrors.New(
		"ORGANIZATION_SELECTOR_INVALID",
		"organization selector must be a valid UUID",
		http.StatusUnprocessableEntity,
	)
}

func resolverInternalError(err error) error {
	return coreerrors.Wrap(
		"TENANT_RESOLUTION_FAILED",
		"failed to resolve organization context",
		http.StatusInternalServerError,
		err,
	)
}
