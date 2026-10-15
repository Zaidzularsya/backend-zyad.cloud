package service

import (
	"context"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"

	"errors"

	"github.com/jackc/pgx/v5"
)

type settingsService struct {
	repo    repository.SettingsRepository
	members MemberDirectory
}

func NewSettingsService(repo repository.SettingsRepository, members MemberDirectory) SettingsService {
	return &settingsService{repo: repo, members: members}
}

func (s *settingsService) Get(ctx context.Context, scope coretenant.Scope) (domain.Settings, error) {
	return s.repo.Get(ctx, scope)
}

func (s *settingsService) Update(ctx context.Context, scope coretenant.Scope, in domain.Settings, userID string) (domain.Settings, error) {
	if in.InvoiceLeadDays < 0 || in.InvoiceLeadDays > 60 {
		return domain.Settings{}, invalidSettings("Hari kirim sebelum periode harus 0–60.")
	}
	if in.PaymentTermsDays < 0 || in.PaymentTermsDays > 90 {
		return domain.Settings{}, invalidSettings("Termin pembayaran harus 0–90 hari.")
	}
	channels, err := normalizeChannels(in.DefaultChannels)
	if err != nil {
		return domain.Settings{}, invalidSettings("Pilih minimal satu kanal default (email atau whatsapp).")
	}
	in.DefaultChannels = channels
	in.DefaultSenderUserID = strings.TrimSpace(in.DefaultSenderUserID)
	if in.DefaultSenderUserID != "" && s.members != nil {
		if _, err := s.members.Find(ctx, scope, in.DefaultSenderUserID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.Settings{}, invalidSettings("Pengirim default harus anggota aktif organisasi ini.")
			}
			return domain.Settings{}, err
		}
	}
	return s.repo.Upsert(ctx, scope, in, userID)
}

func (s *settingsService) Senders(ctx context.Context, scope coretenant.Scope) ([]Member, error) {
	if s.members == nil {
		return []Member{}, nil
	}
	return s.members.Active(ctx, scope)
}
