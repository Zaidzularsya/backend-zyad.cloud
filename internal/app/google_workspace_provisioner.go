package app

import (
	"context"
	"regexp"
	"strings"

	organizationdto "zyad.cloud/internal/modules/organization/dto"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	userservice "zyad.cloud/internal/modules/user/service"
)

var workspaceSlugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type googleWorkspaceProvisioner struct {
	onboarding *organizationservice.OnboardingService
}

func (p googleWorkspaceProvisioner) ProvisionGoogleWorkspace(
	ctx context.Context,
	userID string,
	sessionID string,
	workspaceName string,
	metadata userservice.LoginHistoryRecord,
) error {
	_, err := p.onboarding.CreateWorkspace(
		ctx,
		userID,
		sessionID,
		organizationdto.CreateWorkspaceRequest{
			Name:     workspaceName,
			Slug:     temporaryWorkspaceSlug(workspaceName, userID),
			Timezone: "Asia/Jakarta",
			Locale:   "id-ID",
		},
		organizationservice.OnboardingMetadata{
			IPAddress: metadata.IPAddress,
			UserAgent: metadata.UserAgent,
		},
	)
	return err
}

func temporaryWorkspaceSlug(name string, userID string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = workspaceSlugCleaner.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	if base == "" {
		base = "workspace"
	}
	suffix := strings.ReplaceAll(userID, "-", "")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	if suffix == "" {
		return base
	}
	return strings.Trim(base+"-"+suffix, "-")
}
