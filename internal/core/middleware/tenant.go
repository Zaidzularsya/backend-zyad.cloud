package middleware

import "github.com/gin-gonic/gin"

const TenantIDContextKey = "tenant_id"
const TenantIDHeader = "X-Org-Id"

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
	value, ok := c.Get(TenantIDContextKey)
	if !ok {
		return ""
	}
	tenantID, _ := value.(string)
	return tenantID
}
