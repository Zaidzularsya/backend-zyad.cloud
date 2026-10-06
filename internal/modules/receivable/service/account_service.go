package service

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

type accountService struct{ repo repository.AccountRepository }

func NewAccountService(repo repository.AccountRepository) AccountService {
	return &accountService{repo: repo}
}

func normalizeAccount(in AccountInput, allowSource bool) (repository.AccountParams, error) {
	p := repository.AccountParams{
		Name: strings.TrimSpace(in.Name), CompanyName: strings.TrimSpace(in.CompanyName), Email: strings.TrimSpace(in.Email),
		Phone: strings.TrimSpace(in.Phone), Address: strings.TrimSpace(in.Address),
	}
	if n := utf8.RuneCountInString(p.Name); n < 1 || n > 200 {
		return p, invalidAccount("Nama wajib diisi (1–200 karakter).")
	}
	if utf8.RuneCountInString(p.CompanyName) > 200 {
		return p, invalidAccount("Nama perusahaan maksimal 200 karakter.")
	}
	if utf8.RuneCountInString(p.Phone) > 50 {
		return p, invalidAccount("Telepon maksimal 50 karakter.")
	}
	if p.Email != "" {
		addr, err := mail.ParseAddress(p.Email)
		if err != nil || addr.Address != p.Email || len(p.Email) > 255 {
			return p, invalidAccount("Alamat email tidak valid.")
		}
	}
	if allowSource {
		p.SourceType, p.SourceID = strings.TrimSpace(in.SourceType), strings.TrimSpace(in.SourceID)
		if (p.SourceType == "") != (p.SourceID == "") || (p.SourceType != "" && p.SourceType != domain.SourceTypeCRMContact) {
			return p, invalidAccount("Sumber harus kontak CRM beserta id-nya.")
		}
	}
	return p, nil
}

func (s *accountService) Create(ctx context.Context, scope coretenant.Scope, in AccountInput, userID string) (domain.Account, error) {
	p, err := normalizeAccount(in, true)
	if err != nil {
		return domain.Account{}, err
	}
	return s.repo.Create(ctx, scope, p, userID)
}

func (s *accountService) EnsureForSource(ctx context.Context, scope coretenant.Scope, in AccountInput, userID string) (domain.Account, error) {
	p, err := normalizeAccount(in, true)
	if err != nil {
		return domain.Account{}, err
	}
	if p.SourceID == "" {
		return domain.Account{}, invalidAccount("Sumber wajib diisi.")
	}
	found, err := s.repo.FindBySource(ctx, scope, p.SourceType, p.SourceID)
	if err == nil {
		return found, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, err
	}
	created, err := s.repo.Create(ctx, scope, p, userID)
	if errors.Is(err, repository.ErrDuplicateAccountSource) { // balapan: pembuat lain menang
		return s.repo.FindBySource(ctx, scope, p.SourceType, p.SourceID)
	}
	return created, err
}

func (s *accountService) Update(ctx context.Context, scope coretenant.Scope, id string, in AccountInput) (domain.Account, error) {
	p, err := normalizeAccount(in, false)
	if err != nil {
		return domain.Account{}, err
	}
	acc, err := s.repo.Update(ctx, scope, id, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, receivable.ErrAccountNotFound
	}
	return acc, err
}

func (s *accountService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Account, error) {
	acc, err := s.repo.FindByID(ctx, scope, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, receivable.ErrAccountNotFound
	}
	return acc, err
}

func (s *accountService) List(ctx context.Context, scope coretenant.Scope, search string, limit, offset int) ([]domain.Account, int64, error) {
	return s.repo.List(ctx, scope, strings.TrimSpace(search), limit, offset)
}
