package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

// MemberService menyediakan lookup anggota organization untuk dropdown
// owner/assignee di CRM.
type MemberService interface {
	ListActive(context.Context, coretenant.Scope) ([]domain.OrganizationMember, error)
}
