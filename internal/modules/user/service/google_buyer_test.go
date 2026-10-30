package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/model"
)

func newGoogleBuyerService(t *testing.T, repo *fakeLoginRepository, claims coreauth.GoogleIDTokenClaims) (*AuthService, *fakeGoogleWorkspaceProvisioner) {
	t.Helper()
	svc := newTestAuthService(t, repo)
	svc.googleEnabled = true
	svc.googleAutoRegister = true
	svc.googleDefaultRole = "member"
	svc.googleDefaultStatus = model.UserStatusActive
	svc.googleVerifier = &fakeGoogleVerifier{claims: claims}
	provisioner := &fakeGoogleWorkspaceProvisioner{}
	svc.googleWorkspace = provisioner
	return svc, provisioner
}

func TestResolveGoogleBuyerRegistersWithoutSessionOrPersonalWorkspace(t *testing.T) {
	repo := &fakeLoginRepository{
		findGoogleSubjectErr: ErrGoogleIdentityNotFound,
		findGoogleEmailErr:   ErrGoogleEmailUserNotFound,
		createdGoogleUser:    GoogleAuthUser{ID: "user-new", Name: "Budi", Email: "budi@example.com", Status: model.UserStatusActive},
		sessionID:            "session-should-not-be-used",
	}
	svc, provisioner := newGoogleBuyerService(t, repo, coreauth.GoogleIDTokenClaims{
		Subject: "sub-1", Email: "Budi@Example.com", EmailVerified: true, Name: "Budi",
	})

	buyer, err := svc.ResolveGoogleBuyer(context.Background(), "token", LoginHistoryRecord{IPAddress: "203.0.113.9"})
	if err != nil {
		t.Fatalf("resolve buyer: %v", err)
	}
	if buyer.UserID != "user-new" || buyer.Email != "budi@example.com" || buyer.Name != "Budi" || !buyer.IsNewUser {
		t.Fatalf("buyer = %+v", buyer)
	}
	if repo.createdSession.UserID != "" {
		t.Fatalf("no session may be created, got %+v", repo.createdSession)
	}
	if provisioner.userID != "" {
		t.Fatal("personal workspace must not be provisioned by the tenant checkout path")
	}
}

func TestResolveGoogleBuyerReturnsExistingUser(t *testing.T) {
	repo := &fakeLoginRepository{
		googleUserBySubject: GoogleAuthUser{ID: "user-1", Name: "Ani", Email: "ani@example.com", Status: model.UserStatusActive},
	}
	svc, _ := newGoogleBuyerService(t, repo, coreauth.GoogleIDTokenClaims{Subject: "sub-1", Email: "ani@example.com", EmailVerified: true})

	buyer, err := svc.ResolveGoogleBuyer(context.Background(), "token", LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("resolve buyer: %v", err)
	}
	if buyer.UserID != "user-1" || buyer.IsNewUser {
		t.Fatalf("buyer = %+v", buyer)
	}
}

func TestResolveGoogleBuyerRejectsUnverifiedEmailWith401(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc, _ := newGoogleBuyerService(t, repo, coreauth.GoogleIDTokenClaims{Subject: "sub-1", Email: "x@example.com", EmailVerified: false})

	_, err := svc.ResolveGoogleBuyer(context.Background(), "token", LoginHistoryRecord{})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_GOOGLE_EMAIL_UNVERIFIED" || appErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected 401 AUTH_GOOGLE_EMAIL_UNVERIFIED, got %v", err)
	}
	if repo.googleSubjectLookup != "" {
		t.Fatal("no user lookup may happen for an unverified email")
	}
}

func TestResolveGoogleBuyerRejectsInvalidTokenWith401(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newTestAuthService(t, repo)
	svc.googleEnabled = true
	svc.googleVerifier = &fakeGoogleVerifier{err: coreauth.ErrGoogleIDTokenInvalid}

	_, err := svc.ResolveGoogleBuyer(context.Background(), "bad", LoginHistoryRecord{})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestResolveGoogleBuyerRejectsInactiveUser(t *testing.T) {
	repo := &fakeLoginRepository{
		googleUserBySubject: GoogleAuthUser{ID: "user-1", Email: "a@example.com", Status: model.UserStatusSuspended},
	}
	svc, _ := newGoogleBuyerService(t, repo, coreauth.GoogleIDTokenClaims{Subject: "sub-1", Email: "a@example.com", EmailVerified: true})

	if _, err := svc.ResolveGoogleBuyer(context.Background(), "token", LoginHistoryRecord{}); err == nil {
		t.Fatal("expected inactive user error")
	}
}

func TestResolveGoogleBuyerRejectsDisabledGoogle(t *testing.T) {
	svc := newTestAuthService(t, &fakeLoginRepository{})
	svc.googleVerifier = &fakeGoogleVerifier{}
	_, err := svc.ResolveGoogleBuyer(context.Background(), "token", LoginHistoryRecord{})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "AUTH_GOOGLE_DISABLED" {
		t.Fatalf("expected AUTH_GOOGLE_DISABLED, got %v", err)
	}
}
