package domain

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// ReferralAlphabet has 32 symbols without look-alikes (0/O, 1/I), so a code
// can be read aloud or typed from a screenshot without ambiguity.
const ReferralAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// ReferralCodeLength is fixed by the users.referral_code column (varchar(8)).
const ReferralCodeLength = 8

var (
	// ErrReferralCodeTaken: the generated code already belongs to another user.
	ErrReferralCodeTaken = errors.New("referral code already taken")
	// ErrReferralCodeExhausted: every retry collided; practically unreachable with 32^8 codes.
	ErrReferralCodeExhausted = errors.New("could not allocate a unique referral code")
	// ErrUserNotFound: the user does not exist (referral code owner, or the email staff
	// tried to link has no account; staff should send a claim invite instead).
	ErrUserNotFound = errors.New("user not found")
)

// NewReferralCode draws a code from r (crypto/rand.Reader in production).
// 256 is a multiple of 32, so taking each byte modulo 32 is unbiased.
func NewReferralCode(r io.Reader) (string, error) {
	buf := make([]byte, ReferralCodeLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	out := make([]byte, ReferralCodeLength)
	for i, b := range buf {
		out[i] = ReferralAlphabet[int(b)%len(ReferralAlphabet)]
	}
	return string(out), nil
}

// NormalizeReferralCode accepts codes typed in any case with surrounding spaces.
func NormalizeReferralCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsReferralCode reports whether code (already normalized) has the referral code shape.
func IsReferralCode(code string) bool {
	if len(code) != ReferralCodeLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		if strings.IndexByte(ReferralAlphabet, code[i]) < 0 {
			return false
		}
	}
	return true
}
