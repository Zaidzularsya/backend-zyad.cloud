package middleware

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

const TenantIDContextKey = "tenant_id"
const TenantContextKey = "tenant_context"
const OrganizationIDContextKey = "organization_id"
const OrganizationSlugContextKey = "organization_slug"
const OrganizationTypeContextKey = "organization_type"
const OrganizationStatusContextKey = "organization_status"
const MembershipIDContextKey = "membership_id"
const ResolutionSourceContextKey = "resolution_source"
const DataPlacementContextKey = "data_placement"
const ImpersonationSessionIDContextKey = "impersonation_session_id"
const OrganizationIDHeader = "X-Organization-ID"
const LegacyOrganizationIDHeader = "X-Org-Id"
const LegacyTenantIDHeader = "X-Tenant-ID"

// TenantIDHeader is retained for compatibility. New code must use
// OrganizationIDHeader and validate membership before setting TenantContext.
const TenantIDHeader = LegacyOrganizationIDHeader

type AuthenticatedOrganizationResolver interface {
	ResolveAuthenticatedOrganization(
		context.Context,
		string,
		string,
		string,
		string,
	) (coretenant.Context, bool, error)
}

func Tenant(defaultTenant string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader(TenantIDHeader)
		if tenantID == "" {
			tenantID = defaultTenant
		}
		if tenantID != "" {
			c.Set(TenantIDContextKey, tenantID)
		}
		c.Next()
	}
}

func TenantID(c *gin.Context) string {
	if tenantContext, ok := TenantContext(c); ok {
		return tenantContext.OrganizationID()
	}
	value, ok := c.Get(TenantIDContextKey)
	if !ok {
		return ""
	}
	tenantID, _ := value.(string)
	return tenantID
}

func SetTenantContext(c *gin.Context, tenantContext coretenant.Context) bool {
	if !tenantContext.IsValid() {
		return false
	}

	c.Set(TenantContextKey, tenantContext)
	c.Set(TenantIDContextKey, tenantContext.OrganizationID())
	c.Set(OrganizationIDContextKey, tenantContext.OrganizationID())
	c.Set(OrganizationSlugContextKey, tenantContext.OrganizationSlug())
	c.Set(OrganizationTypeContextKey, string(tenantContext.OrganizationType()))
	c.Set(OrganizationStatusContextKey, string(tenantContext.OrganizationStatus()))
	c.Set(MembershipIDContextKey, tenantContext.MembershipID())
	c.Set(ResolutionSourceContextKey, string(tenantContext.ResolutionSource()))
	c.Set(DataPlacementContextKey, string(tenantContext.DataPlacement()))
	c.Set(ImpersonationSessionIDContextKey, tenantContext.ImpersonationSessionID())
	c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
	return true
}

func TenantContext(c *gin.Context) (coretenant.Context, bool) {
	value, ok := c.Get(TenantContextKey)
	if ok {
		tenantContext, validType := value.(coretenant.Context)
		if validType && tenantContext.IsValid() {
			return tenantContext, true
		}
	}
	return coretenant.FromContext(c.Request.Context())
}

func RequireTenantContext(c *gin.Context) (coretenant.Context, error) {
	tenantContext, ok := TenantContext(c)
	if !ok {
		return coretenant.Context{}, coreerrors.New(
			"TENANT_CONTEXT_REQUIRED",
			"organization context is required",
			http.StatusForbidden,
		)
	}
	return tenantContext, nil
}

func RequireActiveTenant() gin.HandlerFunc {
	return requireTenant(func(tenantContext coretenant.Context) error {
		if !tenantContext.IsActive() {
			return coreerrors.New(
				"ORGANIZATION_NOT_ACTIVE",
				"organization is not active",
				http.StatusForbidden,
			)
		}
		return nil
	})
}

func RequireSetupTenant() gin.HandlerFunc {
	return requireTenant(func(tenantContext coretenant.Context) error {
		switch tenantContext.OrganizationStatus() {
		case coretenant.OrganizationStatusPending,
			coretenant.OrganizationStatusProvisioning,
			coretenant.OrganizationStatusProvisioningFailed:
			return nil
		default:
			return coreerrors.New(
				"ORGANIZATION_SETUP_ONLY",
				"organization is not in setup mode",
				http.StatusConflict,
			)
		}
	})
}

func RequirePlatformTenant() gin.HandlerFunc {
	return requireTenant(func(tenantContext coretenant.Context) error {
		if tenantContext.OrganizationType() != coretenant.OrganizationTypePlatform {
			return coreerrors.New(
				"PLATFORM_ORGANIZATION_REQUIRED",
				"platform organization context is required",
				http.StatusForbidden,
			)
		}
		return nil
	})
}

func RequireCustomerTenant() gin.HandlerFunc {
	return requireTenant(func(tenantContext coretenant.Context) error {
		if tenantContext.OrganizationType() != coretenant.OrganizationTypeCustomer {
			return coreerrors.New(
				"CUSTOMER_ORGANIZATION_REQUIRED",
				"customer organization context is required",
				http.StatusForbidden,
			)
		}
		return nil
	})
}

func TenantLogFields(c *gin.Context) map[string]any {
	tenantContext, ok := TenantContext(c)
	if !ok {
		return map[string]any{}
	}
	fields := map[string]any{
		"request_id":               RequestIDFromContext(c),
		"organization_id":          tenantContext.OrganizationID(),
		"organization_slug":        tenantContext.OrganizationSlug(),
		"organization_type":        tenantContext.OrganizationType(),
		"organization_status":      tenantContext.OrganizationStatus(),
		"membership_id":            tenantContext.MembershipID(),
		"resolution_source":        tenantContext.ResolutionSource(),
		"data_placement":           tenantContext.DataPlacement(),
		"impersonation_session_id": tenantContext.ImpersonationSessionID(),
	}
	if user, authenticated := AuthenticatedUserFromContext(c); authenticated {
		fields["user_id"] = user.ID
		fields["session_id"] = user.SessionID
	}
	return fields
}

func requireTenant(validate func(coretenant.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantContext, err := RequireTenantContext(c)
		if err == nil && validate != nil {
			err = validate(tenantContext)
		}
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

func ResolveAuthenticatedOrganization(
	resolver AuthenticatedOrganizationResolver,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if resolver == nil {
			corehttp.Fail(c, coreerrors.New(
				"TENANT_RESOLVER_REQUIRED",
				"organization resolver is required",
				http.StatusInternalServerError,
			))
			c.Abort()
			return
		}
		user, ok := AuthenticatedUserFromContext(c)
		if !ok || user.ID == "" || user.SessionID == "" {
			corehttp.Fail(c, coreerrors.New(
				"UNAUTHORIZED",
				"authenticated session is required",
				http.StatusUnauthorized,
			))
			c.Abort()
			return
		}

		selector, _ := OrganizationSelector(c)
		tenantContext, resolved, err := resolver.ResolveAuthenticatedOrganization(
			c.Request.Context(),
			user.ID,
			user.SessionID,
			selector,
			c.Request.Host,
		)
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		if resolved && !SetTenantContext(c, tenantContext) {
			corehttp.Fail(c, coreerrors.New(
				"TENANT_CONTEXT_INVALID",
				"resolved organization context is invalid",
				http.StatusInternalServerError,
			))
			c.Abort()
			return
		}
		c.Next()
	}
}

// OrganizationSelector returns an untrusted organization selector candidate.
// A resolver must validate membership and organization status before using it.
func OrganizationSelector(c *gin.Context) (organizationID string, header string) {
	for _, candidate := range []string{
		OrganizationIDHeader,
		LegacyOrganizationIDHeader,
		LegacyTenantIDHeader,
	} {
		if value := strings.TrimSpace(c.GetHeader(candidate)); value != "" {
			return value, candidate
		}
	}
	return "", ""
}
