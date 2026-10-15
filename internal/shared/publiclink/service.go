package publiclink

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
)

type Service struct {
	repo   Repository
	secret string
	now    func() time.Time
}

func NewService(repo Repository, secret string) *Service {
	return &Service{repo: repo, secret: secret, now: func() time.Time { return time.Now().UTC() }}
}

// Ensure mengembalikan link aktif (memperbarui expires_at bila berbeda) atau
// membuat baru; Link.Token selalu terisi.
func (s *Service) Ensure(ctx context.Context, scope coretenant.Scope, docType, docID string, expiresAt time.Time, createdBy string) (Link, error) {
	expiresAt = expiresAt.UTC()
	link, enc, err := s.repo.FindActive(ctx, scope, docType, docID)
	if err == nil {
		return s.reuse(ctx, scope, link, enc, expiresAt)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Link{}, err
	}

	token, hash, err := NewToken()
	if err != nil {
		return Link{}, err
	}
	tokenEnc, err := corecrypto.EncryptSecret(s.secret, token)
	if err != nil {
		return Link{}, err
	}
	link, err = s.repo.Insert(ctx, scope, docType, docID, hash, tokenEnc, expiresAt, createdBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Permintaan bersamaan sudah membuat link aktif: pakai itu.
			link, enc, findErr := s.repo.FindActive(ctx, scope, docType, docID)
			if findErr != nil {
				return Link{}, findErr
			}
			return s.reuse(ctx, scope, link, enc, expiresAt)
		}
		return Link{}, err
	}
	link.Token = token
	return link, nil
}

func (s *Service) reuse(ctx context.Context, scope coretenant.Scope, link Link, enc string, expiresAt time.Time) (Link, error) {
	token, err := corecrypto.DecryptSecret(s.secret, enc)
	if err != nil {
		return Link{}, err
	}
	if !link.ExpiresAt.Equal(expiresAt) {
		if err := s.repo.UpdateExpiry(ctx, scope, link.ID, expiresAt); err != nil {
			return Link{}, err
		}
		link.ExpiresAt = expiresAt
	}
	link.Token = token
	return link, nil
}

func (s *Service) Revoke(ctx context.Context, scope coretenant.Scope, docType, docID string) error {
	return s.repo.Revoke(ctx, scope, docType, docID, s.now())
}

// Resolve: ErrLinkInvalid HANYA bila token tidak dikenal. Link dicabut dan
// kedaluwarsa tetap dikembalikan — pemanggil yang memutuskan state.
func (s *Service) Resolve(ctx context.Context, token string) (Link, error) {
	if token == "" {
		return Link{}, ErrLinkInvalid
	}
	link, err := s.repo.FindByHash(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrLinkInvalid
	}
	return link, err
}

// Touch mencatat akses terakhir; kegagalan tidak boleh mengganggu pembaca.
func (s *Service) Touch(ctx context.Context, link Link) {
	_ = s.repo.Touch(ctx, link.ID, s.now())
}
