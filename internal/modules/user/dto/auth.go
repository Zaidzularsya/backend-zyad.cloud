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

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}
