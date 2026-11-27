package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"zyad.cloud/internal/modules/social/domain"
)

// maxReferralCodeAttempts bounds retries when a fresh code collides with an existing one.
const maxReferralCodeAttempts = 5

// AttributionStore is the persistence the attribution service needs.
type AttributionStore interface {
	// ReferrerByCode returns the owner of a referral code.
	ReferrerByCode(ctx context.Context, code string) (userID string, found bool, err error)
	// ReferralCode returns the user's code, or "" when none was assigned yet.
	ReferralCode(ctx context.Context, userID string) (string, error)
	// AssignReferralCode sets the code only when the user has none and returns
	// the code the user ends up with; domain.ErrReferralCodeTaken on collision.
	AssignReferralCode(ctx context.Context, userID, code string) (string, error)
	InsertVisit(ctx context.Context, event domain.VisitEvent) error
	// RecordSignupReferral sets users.referred_by_user_id only when it is still
	// empty and, in the same transaction, writes the signup event. It reports
	// whether the referrer was set.
	RecordSignupReferral(ctx context.Context, referral domain.SignupReferral) (bool, error)
	CountEvents(ctx context.Context, referrerUserID string) (domain.EventCounts, error)
	// RecordBecameCustomer writes the became_customer event of a referred user
	// once; it reports whether an event was written.
	RecordBecameCustomer(ctx context.Context, userID string) (bool, error)
}

// VisitInput is a landing visit through a referral link.
type VisitInput struct {
	Code      string
	Channel   string
	PostID    string
	IP        string
	UserAgent string
}

// Summary is the referrer's own view of their code and its results.
type Summary struct {
	Code     string
	ShareURL string
	Visits   int
	Signups  int
}

type AttributionService struct {
	store        AttributionStore
	shareURLBase string
	random       io.Reader
	now          func() time.Time
}

// NewAttributionService builds the service; shareURLBase is the origin share
// links point to (ShareURL = <base>/r/<code>).
func NewAttributionService(store AttributionStore, shareURLBase string) *AttributionService {
	return &AttributionService{
		store:        store,
		shareURLBase: strings.TrimRight(strings.TrimSpace(shareURLBase), "/"),
		random:       rand.Reader,
		now:          time.Now,
	}
}

// EnsureCode returns the user's referral code, assigning one on first use.
// The code never changes once assigned.
func (s *AttributionService) EnsureCode(ctx context.Context, userID string) (string, error) {
	code, err := s.store.ReferralCode(ctx, userID)
	if err != nil {
		return "", err
	}
	if code != "" {
		return code, nil
	}
	for attempt := 0; attempt < maxReferralCodeAttempts; attempt++ {
		candidate, err := domain.NewReferralCode(s.random)
		if err != nil {
			return "", err
		}
		assigned, err := s.store.AssignReferralCode(ctx, userID, candidate)
		if errors.Is(err, domain.ErrReferralCodeTaken) {
			continue
		}
		if err != nil {
			return "", err
		}
		return assigned, nil
	}
	return "", domain.ErrReferralCodeExhausted
}

// IsKnownCode reports whether code belongs to a user. Malformed codes are simply unknown.
func (s *AttributionService) IsKnownCode(ctx context.Context, code string) (bool, error) {
	_, found, err := s.lookup(ctx, code)
	return found, err
}

// RecordVisit stores a visit for a known code. Unknown or malformed codes are
// ignored without error so the public endpoint never reveals which codes exist
// through its behaviour. The IP is stored only as a SHA-256 hex digest.
func (s *AttributionService) RecordVisit(ctx context.Context, in VisitInput) error {
	referrerID, found, err := s.lookup(ctx, in.Code)
	if err != nil || !found {
		return err
	}
	return s.store.InsertVisit(ctx, domain.VisitEvent{
		ReferralCode:   domain.NormalizeReferralCode(in.Code),
		ReferrerUserID: referrerID,
		Channel:        domain.NormalizeChannel(in.Channel),
		PostID:         domain.NormalizePostID(in.PostID),
		IPHash:         hashIP(in.IP),
		UserAgent:      domain.NormalizeUserAgent(in.UserAgent),
	})
}

// RecordSignup credits the referrer of a newly registered user. First touch
// wins: a code first seen more than 30 days ago, an unknown code, or the
// user's own code are ignored, and an existing referrer is never replaced.
// A zero FirstSeenAt is treated as "seen at sign-up".
func (s *AttributionService) RecordSignup(ctx context.Context, userID string, in *domain.AttributionInput) error {
	if in == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	if !in.FirstSeenAt.IsZero() && s.now().Sub(in.FirstSeenAt) > domain.AttributionWindow {
		return nil
	}
	referrerID, found, err := s.lookup(ctx, in.Code)
	if err != nil || !found {
		return err
	}
	if referrerID == userID {
		return nil
	}
	_, err = s.store.RecordSignupReferral(ctx, domain.SignupReferral{
		SubjectUserID:  userID,
		ReferrerUserID: referrerID,
		ReferralCode:   domain.NormalizeReferralCode(in.Code),
		Channel:        domain.NormalizeChannel(in.Channel),
		PostID:         domain.NormalizePostID(in.PostID),
	})
	return err
}

// RecordBecameCustomer credits the user's referrer (if any) the first time the
// user becomes a customer of some organization.
func (s *AttributionService) RecordBecameCustomer(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	_, err := s.store.RecordBecameCustomer(ctx, userID)
	return err
}

// Summary returns the user's code (assigning it if needed), share link and totals.
func (s *AttributionService) Summary(ctx context.Context, userID string) (Summary, error) {
	code, err := s.EnsureCode(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	counts, err := s.store.CountEvents(ctx, userID)
	if err != nil {
		return Summary{}, fmt.Errorf("count attribution events: %w", err)
	}
	return Summary{
		Code:     code,
		ShareURL: s.shareURLBase + "/r/" + code,
		Visits:   counts.Visits,
		Signups:  counts.Signups,
	}, nil
}

func (s *AttributionService) lookup(ctx context.Context, code string) (string, bool, error) {
	code = domain.NormalizeReferralCode(code)
	if !domain.IsReferralCode(code) {
		return "", false, nil
	}
	return s.store.ReferrerByCode(ctx, code)
}

func hashIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:])
}
