package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreauth "zyad.cloud/internal/core/auth"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"
	redisplatform "zyad.cloud/internal/platform/redis"

	"github.com/gin-gonic/gin"
)

const googleAuthCodeTTL = 60 * time.Second

type AuthHandler struct {
	service     *service.AuthService
	redis       *redisplatform.Client
	frontendURL string
}

func NewAuthHandler(authService *service.AuthService, redisClient *redisplatform.Client, frontendURL string) *AuthHandler {
	return &AuthHandler{service: authService, redis: redisClient, frontendURL: strings.TrimRight(frontendURL, "/")}
}

func (h *AuthHandler) RegisterRoutes(router gin.IRoutes) {
	router.POST("/auth/login", h.Login)
	router.POST("/auth/google", h.GoogleAuth)
	router.POST("/auth/google/callback", h.GoogleAuthCallback)
	router.POST("/auth/google/exchange", h.GoogleAuthExchange)
	router.POST("/auth/logout", h.Logout)
	router.POST("/auth/refresh-token", h.RefreshToken)
	router.POST("/auth/forgot-password", h.ForgotPassword)
	router.POST("/auth/reset-password/validate", h.ValidateResetToken)
	router.POST("/auth/reset-password", h.ResetPassword)
	router.GET("/auth/me", h.CurrentUser)
	router.POST("/auth/verify-email", h.VerifyEmail)
	router.POST("/auth/resend-verification-email", h.ResendVerificationEmail)
}

func (h *AuthHandler) RegisterProtectedRoutes(router gin.IRoutes) {
	router.POST("/auth/change-password", h.ChangePassword)
	router.GET("/auth/sessions", h.ListSessions)
	router.DELETE("/auth/sessions/:id", h.RevokeSession)
	router.POST("/auth/logout-all", h.LogoutAll)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	result, err := h.service.Login(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		DeviceName: req.DeviceName,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "login successful", result)
}

func (h *AuthHandler) GoogleAuth(c *gin.Context) {
	var req dto.GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	result, err := h.service.GoogleAuth(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		DeviceName: req.DeviceName,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "google authentication successful", result)
}

// GoogleAuthCallback is the GIS `login_uri` target for the redirect (ux_mode:
// 'redirect') sign-in flow: Google POSTs the credential directly here (full
// page navigation, no JS on the SPA side), so the result can't be handed back
// via a JSON response. Instead we store it behind a short-lived one-time code
// in Redis and redirect the browser back to the SPA, which exchanges the code
// via GoogleAuthExchange.
func (h *AuthHandler) GoogleAuthCallback(c *gin.Context) {
	credential := c.PostForm("credential")
	csrfCookie, _ := c.Cookie("g_csrf_token")
	csrfBody := c.PostForm("g_csrf_token")

	if credential == "" || csrfCookie == "" || csrfBody == "" || csrfCookie != csrfBody {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?google_error=csrf")
		return
	}

	// Google's redirect (ux_mode: 'redirect') POST only carries `credential`
	// and `g_csrf_token` — there's no custom state passthrough for the GIS
	// ID-token flow, unlike the separate OAuth2 code-client API. The frontend
	// keeps the post-login destination in sessionStorage across the round
	// trip instead, so the session here always uses the persistent (remember
	// me) TTL, matching common "sign in with Google" UX.
	req := dto.GoogleAuthRequest{
		IDToken:    credential,
		RememberMe: true,
		DeviceName: "Web Browser",
	}

	result, err := h.service.GoogleAuth(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		DeviceName: req.DeviceName,
	})
	if err != nil {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?google_error=1")
		return
	}

	code, err := h.storeGoogleAuthResult(c.Request.Context(), result)
	if err != nil {
		c.Redirect(http.StatusFound, h.frontendURL+"/login?google_error=1")
		return
	}

	landing := url.URL{Path: "/auth/google/callback", RawQuery: url.Values{"code": {code}}.Encode()}
	c.Redirect(http.StatusFound, h.frontendURL+landing.String())
}

// GoogleAuthExchange lets the SPA trade the one-time code minted by
// GoogleAuthCallback for the real access/refresh tokens.
func (h *AuthHandler) GoogleAuthExchange(c *gin.Context) {
	var req dto.GoogleExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	ctx := c.Request.Context()
	key := googleAuthCodeKey(req.Code)
	raw, err := h.redis.Get(ctx, key).Result()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("AUTH_GOOGLE_CODE_INVALID", "google authentication code is invalid or expired", http.StatusUnauthorized))
		return
	}
	h.redis.Del(ctx, key)

	var result dto.GoogleAuthResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		corehttp.Fail(c, coreerrors.New("AUTH_GOOGLE_CODE_INVALID", "google authentication code is invalid or expired", http.StatusUnauthorized))
		return
	}

	corehttp.OK(c, "google authentication successful", result)
}

func (h *AuthHandler) storeGoogleAuthResult(ctx context.Context, result dto.GoogleAuthResponse) (string, error) {
	code, err := coreauth.NewRandomToken(24)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := h.redis.Set(ctx, googleAuthCodeKey(code), payload, googleAuthCodeTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

func googleAuthCodeKey(code string) string {
	return "google-auth-code:" + code
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authorization bearer token is required", 401))
		return
	}

	if err := h.service.Logout(c.Request.Context(), token, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "logout successful", nil)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	result, err := h.service.RefreshToken(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "token refreshed successfully", result)
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	if err := h.service.ForgotPassword(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "if the email is registered, password reset instructions will be sent", nil)
}

func (h *AuthHandler) ValidateResetToken(c *gin.Context) {
	var req dto.ValidateResetTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	result, err := h.service.ValidateResetToken(c.Request.Context(), req)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "reset token is valid", result)
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	if err := h.service.ResetPassword(c.Request.Context(), req); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "password reset successfully", nil)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return
	}

	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), user.ID, user.SessionID, req, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "password changed successfully", nil)
}

func (h *AuthHandler) CurrentUser(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authorization bearer token is required", 401))
		return
	}

	result, err := h.service.CurrentUser(c.Request.Context(), token)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "current user retrieved", result)
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func (h *AuthHandler) ListSessions(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return
	}

	sessions, err := h.service.ListSessions(c.Request.Context(), user.ID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	responses := make([]dto.SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		var lastUsedAtStr *string
		if s.LastUsedAt != nil {
			formatted := s.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
			lastUsedAtStr = &formatted
		}

		responses = append(responses, dto.SessionResponse{
			ID:         s.ID,
			DeviceName: s.DeviceName,
			IPAddress:  s.IPAddress,
			LastUsedAt: lastUsedAtStr,
			ExpiresAt:  s.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
			IsCurrent:  s.ID == user.SessionID,
		})
	}

	corehttp.OK(c, "sessions retrieved successfully", responses)
}

func (h *AuthHandler) RevokeSession(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return
	}

	sessionID := c.Param("id")
	if err := h.service.RevokeSession(c.Request.Context(), sessionID, user.ID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "session revoked successfully", nil)
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return
	}

	var req dto.LogoutAllRequest
	// We bind JSON but make it optional to parse (default to false if not provided)
	_ = c.ShouldBindJSON(&req)

	excludeSessionID := ""
	if req.ExcludeCurrent {
		excludeSessionID = user.SessionID
	}

	if err := h.service.LogoutAll(c.Request.Context(), user.ID, excludeSessionID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "logout all sessions successfully", nil)
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req dto.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.service.VerifyEmail(c.Request.Context(), req); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "email verified successfully", nil)
}

func (h *AuthHandler) ResendVerificationEmail(c *gin.Context) {
	var req dto.ResendVerificationEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.service.ResendVerificationEmail(c.Request.Context(), req); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "if the email is pending verification, a verification email has been sent", nil)
}
