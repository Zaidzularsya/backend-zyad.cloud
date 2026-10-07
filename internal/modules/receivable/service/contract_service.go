package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

type ContractService interface {
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Contract, error)
	List(ctx context.Context, scope coretenant.Scope, f repository.ContractListFilter) ([]domain.Contract, int64, error)
	// SetEndDate: endDate nil menghapus tanggal akhir; selain itu harus >= hari ini (WIB).
	SetEndDate(ctx context.Context, scope coretenant.Scope, id string, endDate *time.Time, userID string) (domain.Contract, error)
	// End: alasan 1–500 karakter; endDate nol = hari ini (WIB).
	End(ctx context.Context, scope coretenant.Scope, id string, endDate time.Time, reason, userID string) (domain.Contract, error)
}

type contractService struct {
	repo      repository.ContractRepository
	listeners *Registry
	now       func() time.Time
}

func NewContractService(repo repository.ContractRepository, listeners *Registry, now func() time.Time) ContractService {
	if now == nil {
		now = time.Now
	}
	return &contractService{repo: repo, listeners: listeners, now: now}
}

func (s *contractService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Contract, error) {
	c, err := s.repo.FindByID(ctx, scope, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contract{}, receivable.ErrContractNotFound
	}
	return c, err
}

func (s *contractService) List(ctx context.Context, scope coretenant.Scope, f repository.ContractListFilter) ([]domain.Contract, int64, error) {
	f.Search = strings.TrimSpace(f.Search)
	return s.repo.List(ctx, scope, f)
}

func (s *contractService) SetEndDate(ctx context.Context, scope coretenant.Scope, id string, endDate *time.Time, userID string) (domain.Contract, error) {
	c, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Contract{}, err
	}
	if endDate != nil {
		d := pricingDay(*endDate)
		if d.Before(businesstime.DayOf(s.now())) {
			return domain.Contract{}, invalidContract("Tanggal akhir tidak boleh sebelum hari ini.")
		}
		if d.Before(c.StartDate) {
			return domain.Contract{}, invalidContract("Tanggal akhir tidak boleh sebelum tanggal mulai.")
		}
		endDate = &d
	}
	return s.mapNotFound(s.repo.SetEndDate(ctx, scope, id, endDate, userID))
}

func (s *contractService) End(ctx context.Context, scope coretenant.Scope, id string, endDate time.Time, reason, userID string) (domain.Contract, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 1 || n > 500 {
		return domain.Contract{}, invalidContract("Alasan wajib diisi (1–500 karakter).")
	}
	c, err := s.Get(ctx, scope, id)
	if err != nil {
		return domain.Contract{}, err
	}
	d := businesstime.DayOf(s.now())
	if !endDate.IsZero() {
		d = pricingDay(endDate)
	}
	if d.Before(c.StartDate) {
		return domain.Contract{}, invalidContract("Tanggal akhir tidak boleh sebelum tanggal mulai.")
	}
	ended, err := s.mapNotFound(s.repo.End(ctx, scope, id, d, reason, userID))
	if err != nil {
		return domain.Contract{}, err
	}
	// Repository hanya mengubah contract berstatus active, jadi event ini tepat sekali per contract.
	s.listeners.ContractEnded(ctx, scope, contractRef(ended))
	return ended, nil
}

func (s *contractService) mapNotFound(c domain.Contract, err error) (domain.Contract, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contract{}, receivable.ErrContractNotFound
	}
	return c, err
}

func pricingDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func contractRef(c domain.Contract) ContractRef {
	return ContractRef{ID: c.ID, Number: c.ContractNumber, SourceID: c.SourceID, SourceType: c.SourceType}
}
