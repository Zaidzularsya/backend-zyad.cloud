package dto

type LoginRequest struct {
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"remember_me"`
	DeviceName string `json:"device_name"`
}

type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	User         AuthUserView `json:"user"`
}

type GoogleAuthRequest struct {
	IDToken     string `json:"id_token" binding:"required"`
	RememberMe  bool   `json:"remember_me"`
	DeviceName  string `json:"device_name"`
	InviteToken string `json:"invite_token,omitempty"`
}

type GoogleAuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int64        `json:"expires_in"`
	IsNewUser    bool         `json:"is_new_user"`
	User         AuthUserView `json:"user"`
}

type AuthUserView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Username    string   `json:"username,omitempty"`
	Status      string   `json:"status"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type CurrentUserResponse struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Email               string             `json:"email"`
	Username            string             `json:"username,omitempty"`
	Phone               string             `json:"phone,omitempty"`
	Status              string             `json:"status"`
	EmailVerifiedAt     *string            `json:"email_verified_at"`
	PhoneVerifiedAt     *string            `json:"phone_verified_at"`
	Profile             CurrentUserProfile `json:"profile"`
	Roles               []string           `json:"roles"`
	Permissions         []string           `json:"permissions"`
	CurrentOrganization any                `json:"current_organization"`
	Session             CurrentSession     `json:"session"`
}

type CurrentUserProfile struct {
	AvatarURL  string `json:"avatar_url,omitempty"`
	Bio        string `json:"bio,omitempty"`
	JobTitle   string `json:"job_title,omitempty"`
	Department string `json:"department,omitempty"`
	Company    string `json:"company,omitempty"`
	Address    string `json:"address,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Language   string `json:"language,omitempty"`
}

type CurrentSession struct {
	ID         string  `json:"id"`
	DeviceName string  `json:"device_name,omitempty"`
	IPAddress  string  `json:"ip_address,omitempty"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	ExpiresAt  string  `json:"expires_at"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ValidateResetTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

type ValidateResetTokenResponse struct {
	Valid     bool   `json:"valid"`
	ExpiresAt string `json:"expires_at"`
}

type ResetPasswordRequest struct {
	Token                string `json:"token" binding:"required"`
	Password             string `json:"password" binding:"required"`
	PasswordConfirmation string `json:"password_confirmation" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword         string `json:"current_password" binding:"required"`
	NewPassword             string `json:"new_password" binding:"required"`
	NewPasswordConfirmation string `json:"new_password_confirmation" binding:"required"`
	LogoutOtherDevices      bool   `json:"logout_other_devices"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type SessionResponse struct {
	ID         string  `json:"id"`
	DeviceName string  `json:"device_name,omitempty"`
	IPAddress  string  `json:"ip_address,omitempty"`
	UserAgent  string  `json:"user_agent,omitempty"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	ExpiresAt  string  `json:"expires_at"`
	IsCurrent  bool    `json:"is_current"`
}

type LogoutAllRequest struct {
	ExcludeCurrent bool `json:"exclude_current"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type ResendVerificationEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
}
