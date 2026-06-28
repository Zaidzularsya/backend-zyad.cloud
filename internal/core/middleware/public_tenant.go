package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

type PublicHostResolver interface {
	ResolvePublicHost(context.Context, string) (coretenant.Context, bool, error)
}

type PublicHostOptions struct {
	TrustForwardedHost bool
	TrustedProxyCIDRs  []string
}

func ResolvePublicOrganization(
	resolver PublicHostResolver,
	options PublicHostOptions,
) (gin.HandlerFunc, error) {
	trustedProxies, err := parseTrustedProxyCIDRs(options.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}

	return func(c *gin.Context) {
		if resolver == nil {
			corehttp.Fail(c, coreerrors.New(
				"PUBLIC_HOST_RESOLVER_REQUIRED",
				"public host resolver is required",
				http.StatusInternalServerError,
			))
			c.Abort()
			return
		}
		if selector, _ := OrganizationSelector(c); selector != "" {
			corehttp.Fail(c, coreerrors.New(
				"PUBLIC_ORGANIZATION_SELECTOR_FORBIDDEN",
				"public requests cannot select an organization by header",
				http.StatusBadRequest,
			))
			c.Abort()
			return
		}

		host := c.Request.Host
		if options.TrustForwardedHost && remoteIPTrusted(c.Request.RemoteAddr, trustedProxies) {
			if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
				host, err = canonicalForwardedHost(forwardedHost)
				if err != nil {
					corehttp.Fail(c, err)
					c.Abort()
					return
				}
			}
		}
		tenantContext, resolved, err := resolver.ResolvePublicHost(
			c.Request.Context(),
			host,
		)
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		if !resolved {
			corehttp.Fail(c, coreerrors.New(
				"PUBLIC_HOST_NOT_FOUND",
				"public host is not registered",
				http.StatusNotFound,
			))
			c.Abort()
			return
		}
		if resolved && !SetTenantContext(c, tenantContext) {
			corehttp.Fail(c, coreerrors.New(
				"TENANT_CONTEXT_INVALID",
				"resolved public tenant context is invalid",
				http.StatusInternalServerError,
			))
			c.Abort()
			return
		}
		c.Next()
	}, nil
}

func canonicalForwardedHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" ||
		strings.ContainsAny(host, "/\\@, \t\r\n") ||
		strings.Contains(host, "://") {
		return "", coreerrors.New(
			"FORWARDED_HOST_INVALID",
			"forwarded host is invalid",
			http.StatusBadRequest,
		)
	}
	return host, nil
}

func parseTrustedProxyCIDRs(values []string) ([]*net.IPNet, error) {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy CIDR %q: %w", value, err)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func remoteIPTrusted(remoteAddr string, networks []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
