package logger

import (
	"log/slog"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
)

type TenantFieldContext struct {
	RequestID       string
	UserID          string
	SessionID       string
	OperatorUserID  string
	EffectiveUserID string
}

func TenantAttrs(
	tenantContext coretenant.Context,
	fieldContext TenantFieldContext,
) []slog.Attr {
	if !tenantContext.IsValid() {
		return nil
	}
	attrs := []slog.Attr{
		slog.String("organization_id", tenantContext.OrganizationID()),
		slog.String("organization_slug", tenantContext.OrganizationSlug()),
		slog.String("organization_type", string(tenantContext.OrganizationType())),
		slog.String("organization_status", string(tenantContext.OrganizationStatus())),
		slog.String("resolution_source", string(tenantContext.ResolutionSource())),
		slog.String("data_placement", string(tenantContext.DataPlacement())),
	}
	appendString := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			attrs = append(attrs, slog.String(key, strings.TrimSpace(value)))
		}
	}
	appendString("request_id", fieldContext.RequestID)
	appendString("membership_id", tenantContext.MembershipID())
	appendString("session_id", fieldContext.SessionID)
	appendString("user_id", fieldContext.UserID)
	appendString("operator_user_id", actorOrDefault(fieldContext.OperatorUserID, fieldContext.UserID))
	appendString("effective_user_id", actorOrDefault(fieldContext.EffectiveUserID, fieldContext.UserID))
	appendString("impersonation_session_id", tenantContext.ImpersonationSessionID())
	return attrs
}

func WithTenant(
	logger *slog.Logger,
	tenantContext coretenant.Context,
	fieldContext TenantFieldContext,
) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	attrs := TenantAttrs(tenantContext, fieldContext)
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	return logger.With(args...)
}

func actorOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}
