package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
)

// UsernameChangeCooldown adalah jeda minimum antar penggantian username oleh user sendiri.
const UsernameChangeCooldown = 30 * 24 * time.Hour

var (
	ErrUsernameInvalid  = errors.New("username is invalid")
	ErrUsernameReserved = errors.New("username is reserved")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,28}[a-z0-9]$`)
var digitsOnlyPattern = regexp.MustCompile(`^[0-9]+$`)

// builtinReservedUsernames digabung dengan PLATFORM_RESERVED_SUBDOMAINS.
var builtinReservedUsernames = []string{
	"admin", "administrator", "root", "system", "support", "help", "zyad", "api", "www", "app",
	"platform", "superadmin", "super_admin", "me", "null", "undefined", "owner", "billing", "security",
}

// NormalizeUsername menormalkan ke huruf kecil lalu memvalidasi pola dan kata cadangan.
func NormalizeUsername(raw string, reserved []string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(username) || strings.Contains(username, "..") || digitsOnlyPattern.MatchString(username) {
		return "", ErrUsernameInvalid
	}
	for _, word := range builtinReservedUsernames {
		if username == word {
			return "", ErrUsernameReserved
		}
	}
	for _, word := range reserved {
		if username == strings.ToLower(strings.TrimSpace(word)) {
			return "", ErrUsernameReserved
		}
	}
	return username, nil
}

func usernameAppError(err error) error {
	switch {
	case errors.Is(err, ErrUsernameReserved):
		return coreerrors.New("USERNAME_RESERVED", "username is reserved", http.StatusUnprocessableEntity)
	default:
		return coreerrors.New("USERNAME_INVALID", "username must be 3-30 characters of letters, digits, dot, underscore or hyphen, start and end with a letter or digit, and not be only digits", http.StatusUnprocessableEntity)
	}
}
