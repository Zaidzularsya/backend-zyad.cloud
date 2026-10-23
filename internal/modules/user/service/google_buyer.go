package service

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/model"
)

// GoogleBuyer adalah user hasil verifikasi kredensial Google untuk checkout publik tenant.
type GoogleBuyer struct {
	UserID    string
	Name      string
	Email     string
	IsNewUser bool
}

// ResolveGoogleBuyer memverifikasi ID token Google lalu mencari atau membuat user dengan aturan
// yang sama dengan GoogleAuth (cari per subject, tautkan per email bila diizinkan, daftar otomatis
// bila diizinkan). Berbeda dari GoogleAuth, jalur ini TIDAK membuat sesi/token dan TIDAK
// memprovisikan workspace pribadi: checkout publik tenant membuat workspace pembelinya sendiri,
// dan pembeli masuk ke aplikasi lewat login Google biasa.
// email_verified=false ditolak 401 (kredensial tidak cukup untuk endpoint publik ini).
func (s *AuthService) ResolveGoogleBuyer(ctx context.Context, idToken string, metadata LoginHistoryRecord) (GoogleBuyer, error) {
	claims, err := s.verifyGoogleToken(ctx, idToken, metadata)
	if err != nil {
		return GoogleBuyer{}, err
	}
	if !claims.EmailVerified {
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			Identifier: strings.ToLower(claims.Email),
			Event:      model.LoginEventFailedLogin,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     "google_email_unverified",
		})
		return GoogleBuyer{}, coreerrors.New("AUTH_GOOGLE_EMAIL_UNVERIFIED", "google email is not verified", http.StatusUnauthorized)
	}

	user, isNewUser, err := s.resolveGoogleUser(ctx, claims, metadata, s.now().UTC())
	if err != nil {
		return GoogleBuyer{}, err
	}
	if !googleUserCanLogin(user) {
		s.recordLoginAttempt(ctx, LoginHistoryRecord{
			UserID:     user.ID,
			Identifier: strings.ToLower(user.Email),
			Event:      model.LoginEventFailedLogin,
			Success:    false,
			IPAddress:  metadata.IPAddress,
			UserAgent:  metadata.UserAgent,
			DeviceName: metadata.DeviceName,
			Reason:     string(user.Status),
		})
		return GoogleBuyer{}, inactiveUserError(user.Status)
	}
	return GoogleBuyer{UserID: user.ID, Name: firstNonEmpty(user.Name, user.Email), Email: user.Email, IsNewUser: isNewUser}, nil
}
