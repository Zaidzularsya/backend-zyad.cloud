package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

// MemberRepository membaca anggota aktif organization dari scope. Read-only;
// pengelolaan membership tetap milik modul organization.
type MemberRepository interface {
	ListActive(context.Context, coretenant.Scope) ([]domain.OrganizationMember, error)
	IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error)
}
