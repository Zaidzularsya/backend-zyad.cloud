package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// AttributionWindow: a referral code first seen longer ago than this no longer
// earns the referrer a sign-up (first-touch, 30 days).
const AttributionWindow = 30 * 24 * time.Hour

// Event values of attribution_events.event.
const (
	EventVisit          = "visit"
	EventSignup         = "signup"
	EventBecameCustomer = "became_customer"
)

// Column limits of attribution_events.
const (
	maxChannelLength   = 20
	maxUserAgentLength = 200
)

// AttributionInput is the first-touch referral context captured by the client
// and handed over at sign-up.
type AttributionInput struct {
	Code        string
	Channel     string
	PostID      string
	FirstSeenAt time.Time
}

// VisitEvent is one "visit" row for a known referral code.
type VisitEvent struct {
	ReferralCode   string
	ReferrerUserID string
	Channel        string
	PostID         string
	IPHash         string
	UserAgent      string
}

// SignupReferral records that SubjectUserID signed up through ReferrerUserID's code.
type SignupReferral struct {
	SubjectUserID  string
	ReferrerUserID string
	ReferralCode   string
	Channel        string
	PostID         string
}

// EventCounts are a referrer's totals per event.
type EventCounts struct {
	Visits  int
	Signups int
}

// NormalizeChannel trims and lower-cases a client-supplied channel and cuts it
// to the column width; it is a free-form label, not a closed enum.
func NormalizeChannel(channel string) string {
	return truncateRunes(strings.ToLower(strings.TrimSpace(sanitizeText(channel))), maxChannelLength)
}

// NormalizePostID keeps a post id only when it is a UUID (the column type);
// anything else is dropped rather than failing the event.
func NormalizePostID(postID string) string {
	postID = strings.ToLower(strings.TrimSpace(postID))
	if !uuidPattern.MatchString(postID) {
		return ""
	}
	return postID
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeUserAgent cuts the user agent to the column width.
func NormalizeUserAgent(userAgent string) string {
	return truncateRunes(strings.TrimSpace(sanitizeText(userAgent)), maxUserAgentLength)
}

// sanitizeText makes client-supplied text storable in a Postgres text column:
// invalid UTF-8 and NUL bytes are rejected by Postgres, so both are dropped
// before any truncation.
func sanitizeText(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, ""), "\x00", "")
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
