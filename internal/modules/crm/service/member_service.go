package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type memberService struct {
	repo repository.MemberRepository
}

func NewMemberService(repo repository.MemberRepository) MemberService {
	return &memberService{repo: repo}
}

func (s *memberService) ListActive(ctx context.Context, scope coretenant.Scope) ([]domain.OrganizationMember, error) {
	return s.repo.ListActive(ctx, scope)
}
