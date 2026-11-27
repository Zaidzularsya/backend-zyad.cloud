package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/social/domain"
)

// domainErrors maps social sentinel errors to their HTTP contract.
var domainErrors = []struct {
	err    error
	code   string
	msg    string
	status int
}{
	{domain.ErrClaimNotAvailable, "CLAIM_NOT_AVAILABLE", "Klaim pelanggan tidak tersedia.", http.StatusNotFound},
	{domain.ErrCustomerLinkExists, "CUSTOMER_LINK_EXISTS", "Kontak ini sudah tertaut ke akun.", http.StatusConflict},
	{domain.ErrCustomerLinkNotFound, "CUSTOMER_LINK_NOT_FOUND", "Tautan pelanggan tidak ditemukan.", http.StatusNotFound},
	{domain.ErrUserNotFound, "USER_NOT_FOUND", "Pengguna dengan email tersebut belum terdaftar. Kirim undangan klaim ke kontak ini.", http.StatusNotFound},
	{domain.ErrInviteInvalid, "CLAIM_INVITE_INVALID", "Undangan tidak valid atau sudah dipakai.", http.StatusNotFound},
	{domain.ErrInviteExpired, "CLAIM_INVITE_EXPIRED", "Undangan sudah kedaluwarsa.", http.StatusGone},
	{domain.ErrContactEmailMissing, "CONTACT_EMAIL_REQUIRED", "Kontak belum memiliki email.", http.StatusUnprocessableEntity},
	{domain.ErrEmailNotVerified, "EMAIL_NOT_VERIFIED", "Verifikasi email Anda terlebih dahulu.", http.StatusForbidden},
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isUUID guards ids that reach `::uuid` casts: a malformed value would otherwise
// surface as a database error (500).
func isUUID(s string) bool { return uuidPattern.MatchString(s) }

func failSocial(c *gin.Context, op string, err error) {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		corehttp.Fail(c, appErr)
		return
	}
	for _, d := range domainErrors {
		if errors.Is(err, d.err) {
			corehttp.Fail(c, coreerrors.New(d.code, d.msg, d.status))
			return
		}
	}
	slog.ErrorContext(c.Request.Context(), "social request failed", "op", op, "error", err)
	corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError))
}

func requireUser(c *gin.Context) (middleware.AuthenticatedUser, bool) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return middleware.AuthenticatedUser{}, false
	}
	return user, true
}
