package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/model"
)

func (r *fakeLoginRepository) CreatePasswordUser(_ context.Context, registration PasswordUserRegistration) (LoginUser, error) {
	r.passwordRegistration = registration
	if r.createPasswordErr != nil {
		return LoginUser{}, r.createPasswordErr
	}
	return r.createdPasswordUser, nil
}

type fakeSignupHooks struct {
	calls       int
	userID      string
	attribution *dto.AttributionInput
}

func (h *fakeSignupHooks) OnUserRegistered(_ context.Context, userID string, attribution *dto.AttributionInput) {
	h.calls++
	h.userID = userID
	h.attribution = attribution
}

func newRegisterService(t *testing.T, repo *fakeLoginRepository) *AuthService {
	t.Helper()
	svc := newTestAuthService(t, repo)
	svc.passwordRegisterOn = true
	svc.googleDefaultRole = "member"
	if repo.createdPasswordUser.ID == "" {
		repo.createdPasswordUser = LoginUser{
			ID: "user-new", Name: "Budi", Email: "budi@x.test", Username: "budi",
			Status: model.UserStatusActive, Roles: []string{"member"},
		}
	}
	repo.sessionID = "session-1"
	return svc
}

func validRegisterRequest() dto.RegisterRequest {
	return dto.RegisterRequest{Name: "Budi", Email: "budi@x.test", Password: "secret-password", Intent: "social"}
}

func TestRegisterCreatesActiveUnverifiedUserAndSendsVerification(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	publisher := &fakeNotificationPublisher{}
	svc.SetNotificationPublisher(publisher)
	provisioner := &fakeGoogleWorkspaceProvisioner{}
	svc.SetGoogleWorkspaceProvisioner(provisioner)

	res, err := svc.Register(context.Background(), validRegisterRequest(), LoginHistoryRecord{IPAddress: "1.2.3.4"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" || !res.IsNewUser {
		t.Fatalf("unexpected response: %+v", res)
	}
	if repo.passwordRegistration.RoleSlug != "member" || repo.passwordRegistration.PasswordHash == "" ||
		repo.passwordRegistration.PasswordHash == "secret-password" {
		t.Fatalf("unexpected registration: %+v", repo.passwordRegistration)
	}
	if !publisher.called || publisher.event.Type != emailVerificationRequestedEvent || publisher.event.UserID != "user-new" {
		t.Fatalf("verification event not published: %+v", publisher.event)
	}
	if repo.emailVerificationToken.UserID != "user-new" || repo.emailVerificationToken.TokenHash == "" {
		t.Fatalf("verification token not stored: %+v", repo.emailVerificationToken)
	}
	if provisioner.userID != "" {
		t.Fatalf("workspace provisioned for social intent")
	}
}

func TestRegisterProvisionsWorkspaceForOperatorIntent(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	provisioner := &fakeGoogleWorkspaceProvisioner{}
	svc.SetGoogleWorkspaceProvisioner(provisioner)

	req := validRegisterRequest()
	req.Intent = "" // default = operator
	if _, err := svc.Register(context.Background(), req, LoginHistoryRecord{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if provisioner.userID != "user-new" || provisioner.sessionID != "session-1" {
		t.Fatalf("workspace not provisioned: %+v", provisioner)
	}
}

func assertCoreError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var ce *coreerrors.AppError
	if !errors.As(err, &ce) || ce.Code != code || ce.Status != status {
		t.Fatalf("error = %v, want %s/%d", err, code, status)
	}
}

func TestRegisterRejectsWhenDisabled(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	svc.passwordRegisterOn = false

	_, err := svc.Register(context.Background(), validRegisterRequest(), LoginHistoryRecord{})
	assertCoreError(t, err, "AUTH_REGISTRATION_DISABLED", http.StatusForbidden)
}

func TestRegisterRejectsShortPassword(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	req := validRegisterRequest()
	req.Password = "short"

	_, err := svc.Register(context.Background(), req, LoginHistoryRecord{})
	assertCoreError(t, err, "AUTH_PASSWORD_POLICY_FAILED", http.StatusUnprocessableEntity)
	if repo.passwordRegistration.Email != "" {
		t.Fatalf("user created despite short password")
	}
}

func TestRegisterRejectsInvalidIntent(t *testing.T) {
	svc := newRegisterService(t, &fakeLoginRepository{})
	req := validRegisterRequest()
	req.Intent = "admin"

	_, err := svc.Register(context.Background(), req, LoginHistoryRecord{})
	assertCoreError(t, err, "VALIDATION_ERROR", http.StatusUnprocessableEntity)
}

func TestRegisterEmailTaken(t *testing.T) {
	repo := &fakeLoginRepository{createPasswordErr: ErrEmailTaken}
	svc := newRegisterService(t, repo)

	_, err := svc.Register(context.Background(), validRegisterRequest(), LoginHistoryRecord{})
	assertCoreError(t, err, "AUTH_EMAIL_TAKEN", http.StatusConflict)
}

func TestRegisterNormalizesEmail(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	req := validRegisterRequest()
	req.Email = " Budi@X.test "

	if _, err := svc.Register(context.Background(), req, LoginHistoryRecord{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if repo.passwordRegistration.Email != "budi@x.test" || repo.passwordRegistration.UsernameBase != "budi" {
		t.Fatalf("registration = %+v", repo.passwordRegistration)
	}
}

func TestRegisterCallsSignupHooksWithAttribution(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	hooks := &fakeSignupHooks{}
	svc.SetSignupHooks(hooks)
	req := validRegisterRequest()
	req.Attribution = &dto.AttributionInput{Code: "ABCD2345", Channel: "instagram"}

	if _, err := svc.Register(context.Background(), req, LoginHistoryRecord{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if hooks.calls != 1 || hooks.userID != "user-new" || hooks.attribution == nil || hooks.attribution.Code != "ABCD2345" {
		t.Fatalf("hooks = %+v", hooks)
	}
}

func TestRegisterSucceedsWhenSignupHookFails(t *testing.T) {
	// The hook interface has no error return, so a failing implementation can only
	// log; this pins that a hook that does nothing useful never affects the result.
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	svc.SetSignupHooks(&fakeSignupHooks{})

	res, err := svc.Register(context.Background(), validRegisterRequest(), LoginHistoryRecord{})
	if err != nil || res.AccessToken == "" {
		t.Fatalf("Register = %+v, %v", res, err)
	}
}

func TestRegisterSucceedsWhenVerificationEmailFails(t *testing.T) {
	repo := &fakeLoginRepository{}
	svc := newRegisterService(t, repo)
	svc.SetNotificationPublisher(&fakeNotificationPublisher{err: errors.New("queue down")})

	res, err := svc.Register(context.Background(), validRegisterRequest(), LoginHistoryRecord{})
	if err != nil || res.AccessToken == "" {
		t.Fatalf("Register = %+v, %v", res, err)
	}
}

func TestGoogleAuthCallsSignupHooksOnlyForNewUser(t *testing.T) {
	svc, _ := newGoogleIntentService(t)
	hooks := &fakeSignupHooks{}
	svc.SetSignupHooks(hooks)

	_, err := svc.GoogleAuth(context.Background(), dto.GoogleAuthRequest{
		IDToken: "tok", Intent: dto.IntentSocial, Attribution: &dto.AttributionInput{Code: "ABCD2345"},
	}, LoginHistoryRecord{})
	if err != nil {
		t.Fatalf("google auth: %v", err)
	}
	if hooks.calls != 1 || hooks.userID != "user-new" || hooks.attribution == nil || hooks.attribution.Code != "ABCD2345" {
		t.Fatalf("hooks = %+v", hooks)
	}
}
