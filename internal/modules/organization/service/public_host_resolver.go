package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

var publicHostLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type PublicHostResolverStore interface {
	ResolveActiveHost(context.Context, string) (repository.ResolvedDomain, error)
	FindOrganizationByID(context.Context, string) (model.Organization, error)
}

type PublicHostResolver struct {
	store                  PublicHostResolverStore
	platformOrganizationID string
	platformPrimaryDomain  string
}

func NewPublicHostResolver(
	store PublicHostResolverStore,
	platformOrganizationID string,
	platformPrimaryDomain string,
) *PublicHostResolver {
	return &PublicHostResolver{
		store:                  store,
		platformOrganizationID: strings.TrimSpace(platformOrganizationID),
		platformPrimaryDomain:  canonicalPublicHost(platformPrimaryDomain),
	}
}

func (r *PublicHostResolver) ResolvePublicHost(
	ctx context.Context,
	rawHost string,
) (coretenant.Context, bool, error) {
	host, err := NormalizePublicHost(rawHost)
	if err != nil {
		return coretenant.Context{}, false, err
	}
	if r.store == nil {
		return coretenant.Context{}, false, coreerrors.New(
			"PUBLIC_HOST_RESOLVER_STORE_REQUIRED",
			"public host resolver store is required",
			http.StatusInternalServerError,
		)
	}

	if r.platformPrimaryDomain != "" && host == r.platformPrimaryDomain {
		if !validUUID(r.platformOrganizationID) {
			return coretenant.Context{}, false, coreerrors.New(
				"PLATFORM_ORGANIZATION_CONFIG_INVALID",
				"platform organization ID is required for the primary domain",
				http.StatusInternalServerError,
			)
		}
		organization, err := r.store.FindOrganizationByID(ctx, r.platformOrganizationID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return coretenant.Context{}, false, nil
			}
			return coretenant.Context{}, false, publicHostInternalError(err)
		}
		if !organization.IsPlatform() || !organization.IsActive() {
			return coretenant.Context{}, false, nil
		}
		return newPublicTenantContext(
			organization,
			coretenant.ResolutionSourcePlatformHost,
			host,
		)
	}

	resolved, err := r.store.ResolveActiveHost(ctx, host)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coretenant.Context{}, false, nil
		}
		return coretenant.Context{}, false, publicHostInternalError(err)
	}

	source, ok := publicDomainResolutionSource(resolved.Domain.Type)
	if !ok || !resolved.Domain.CanResolvePublicly() || !resolved.Organization.IsActive() {
		return coretenant.Context{}, false, nil
	}
	return newPublicTenantContext(resolved.Organization, source, host)
}

func NormalizePublicHost(rawHost string) (string, error) {
	rawHost = strings.TrimSpace(rawHost)
	if rawHost == "" ||
		strings.ContainsAny(rawHost, "/\\@, \t\r\n") ||
		strings.Contains(rawHost, "://") {
		return "", invalidPublicHostError()
	}

	host := rawHost
	if strings.HasPrefix(host, "[") {
		return "", invalidPublicHostError()
	}
	if strings.Contains(host, ":") {
		parsedHost, port, err := net.SplitHostPort(host)
		if err != nil || parsedHost == "" {
			return "", invalidPublicHostError()
		}
		portValue, err := strconv.Atoi(port)
		if err != nil || portValue < 1 || portValue > 65535 {
			return "", invalidPublicHostError()
		}
		host = parsedHost
	}
	host = canonicalPublicHost(host)
	if net.ParseIP(host) != nil || len(host) > 253 {
		return "", invalidPublicHostError()
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", invalidPublicHostError()
	}
	for _, label := range labels {
		if !publicHostLabelPattern.MatchString(label) {
			return "", invalidPublicHostError()
		}
	}
	return host, nil
}

func newPublicTenantContext(
	organization model.Organization,
	source coretenant.ResolutionSource,
	host string,
) (coretenant.Context, bool, error) {
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     organization.ID,
		OrganizationSlug:   organization.Slug,
		OrganizationType:   organization.Type,
		OrganizationStatus: organization.Status,
		ResolutionSource:   source,
		DataPlacement:      organization.DataPlacement,
		RequestHost:        host,
	})
	if err != nil {
		return coretenant.Context{}, false, coreerrors.Wrap(
			"TENANT_CONTEXT_INVALID",
			"resolved public tenant context is invalid",
			http.StatusInternalServerError,
			err,
		)
	}
	return tenantContext, true, nil
}

func publicDomainResolutionSource(domainType model.DomainType) (coretenant.ResolutionSource, bool) {
	switch domainType {
	case model.DomainTypePlatform:
		return coretenant.ResolutionSourcePlatformHost, true
	case model.DomainTypeSubdomain:
		return coretenant.ResolutionSourceSubdomain, true
	case model.DomainTypeCustom:
		return coretenant.ResolutionSourceCustomDomain, true
	default:
		return "", false
	}
}

func canonicalPublicHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func invalidPublicHostError() error {
	return coreerrors.New(
		"PUBLIC_HOST_INVALID",
		"public request host is invalid",
		http.StatusBadRequest,
	)
}

func publicHostInternalError(err error) error {
	return coreerrors.Wrap(
		"PUBLIC_HOST_RESOLUTION_FAILED",
		"failed to resolve public request host",
		http.StatusInternalServerError,
		err,
	)
}
