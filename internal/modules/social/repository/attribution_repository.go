package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database"
)

// AttributionRepository reads/writes users.referral_code, users.referred_by_user_id and
// attribution_events. All three are global (no RLS): the rows belong to users, not tenants.
type AttributionRepository struct {
	db *database.Pool
}

func NewAttributionRepository(db *database.Pool) *AttributionRepository {
	return &AttributionRepository{db: db}
}

func (r *AttributionRepository) ReferrerByCode(ctx context.Context, code string) (string, bool, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		SELECT id::text FROM users
		WHERE referral_code = $1 AND deleted_at IS NULL`, code).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find referrer by code: %w", err)
	}
	return userID, true, nil
}

func (r *AttributionRepository) ReferralCode(ctx context.Context, userID string) (string, error) {
	var code *string
	err := r.db.QueryRow(ctx, `SELECT referral_code FROM users WHERE id = $1::uuid`, userID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read referral code: %w", err)
	}
	if code == nil {
		return "", nil
	}
	return *code, nil
}

// AssignReferralCode writes the code only while the user has none, so concurrent
// first requests agree on one code; the loser reads the winner's code back.
func (r *AttributionRepository) AssignReferralCode(ctx context.Context, userID, code string) (string, error) {
	var assigned string
	err := r.db.QueryRow(ctx, `
		UPDATE users SET referral_code = $2
		WHERE id = $1::uuid AND referral_code IS NULL
		RETURNING referral_code`, userID, code).Scan(&assigned)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_referral_code_unique" {
		return "", domain.ErrReferralCodeTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := r.ReferralCode(ctx, userID)
		if err != nil {
			return "", err
		}
		if existing == "" {
			return "", domain.ErrUserNotFound
		}
		return existing, nil
	}
	if err != nil {
		return "", fmt.Errorf("assign referral code: %w", err)
	}
	return assigned, nil
}

func (r *AttributionRepository) InsertVisit(ctx context.Context, e domain.VisitEvent) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO attribution_events (event, referral_code, referrer_user_id, channel, post_id, ip_hash, user_agent)
		VALUES ($1, $2, $3::uuid, NULLIF($4, ''), NULLIF($5, '')::uuid, NULLIF($6, ''), NULLIF($7, ''))`,
		domain.EventVisit, e.ReferralCode, e.ReferrerUserID, e.Channel, e.PostID, e.IPHash, e.UserAgent)
	if err != nil {
		return fmt.Errorf("insert visit event: %w", err)
	}
	return nil
}

// RecordSignupReferral sets referred_by_user_id once and writes the signup event in
// the same transaction, so an event exists exactly when the referrer was set.
func (r *AttributionRepository) RecordSignupReferral(ctx context.Context, ref domain.SignupReferral) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())

	tag, err := tx.Exec(ctx, `
		UPDATE users SET referred_by_user_id = $2::uuid
		WHERE id = $1::uuid AND referred_by_user_id IS NULL AND id <> $2::uuid`,
		ref.SubjectUserID, ref.ReferrerUserID)
	if err != nil {
		return false, fmt.Errorf("set referrer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO attribution_events (event, referral_code, referrer_user_id, subject_user_id, channel, post_id)
		VALUES ($1, $2, $3::uuid, $4::uuid, NULLIF($5, ''), NULLIF($6, '')::uuid)`,
		domain.EventSignup, ref.ReferralCode, ref.ReferrerUserID, ref.SubjectUserID, ref.Channel, ref.PostID); err != nil {
		return false, fmt.Errorf("insert signup event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit signup referral: %w", err)
	}
	return true, nil
}

func (r *AttributionRepository) CountEvents(ctx context.Context, referrerUserID string) (domain.EventCounts, error) {
	var counts domain.EventCounts
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE event = $2),
			count(*) FILTER (WHERE event = $3)
		FROM attribution_events
		WHERE referrer_user_id = $1::uuid`, referrerUserID, domain.EventVisit, domain.EventSignup).
		Scan(&counts.Visits, &counts.Signups)
	if err != nil {
		return domain.EventCounts{}, fmt.Errorf("count attribution events: %w", err)
	}
	return counts, nil
}
