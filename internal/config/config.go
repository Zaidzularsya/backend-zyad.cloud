package config

import (
	"fmt"
	"sync"
)

type Config struct {
	App          AppConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	Auth         AuthConfig
	SeedAdmin    SeedAdminConfig
	Notification NotificationConfig
	HTTP         HTTPConfig
	MultiTenant  MultiTenantConfig
	Mail         MailConfig
	Mikrotik     MikrotikConfig
	Xendit       XenditConfig
	WhatsApp     WhatsAppConfig
	Discord      DiscordConfig
	Security     SecurityConfig
}

var loadEnvOnce sync.Once

func Load() Config {
	loadEnvOnce.Do(func() {
		_ = loadDefaultDotEnv()
	})

	return Config{
		App:          LoadApp(),
		Database:     LoadDatabase(),
		Redis:        LoadRedis(),
		Auth:         LoadAuth(),
		SeedAdmin:    LoadSeedAdmin(),
		Notification: LoadNotification(),
		HTTP:         LoadHTTP(),
		MultiTenant:  LoadMultiTenant(),
		Mail:         LoadMail(),
		Mikrotik:     LoadMikrotik(),
		Xendit:       LoadXendit(),
		WhatsApp:     LoadWhatsApp(),
		Discord:      LoadDiscord(),
		Security:     LoadSecurity(),
	}
}

type AppConfig struct {
	Name           string
	Host           string
	URL            string
	FrontendURL    string
	Secret         string
	Version        string
	Port           int
	OrganizationID string
	Env            string
}

type DatabaseConfig struct {
	Type                  string
	Host                  string
	Port                  int
	Username              string
	Password              string
	Name                  string
	Schema                string
	SSLMode               string
	ConnectTimeoutSeconds int
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
}

type AuthConfig struct {
	Secret                     string
	ExpiresIn                  string
	RefreshSecret              string
	RefreshExpiresIn           string
	RememberMeRefreshExpiresIn string
	ResetTokenExpiresIn        string
	VerificationTokenExpiresIn string
	OTPExpiresIn               string
	OTPMaxAttempts             int
	PasswordMinLength          int
	CookieSecret               string
	CookieExpiresIn            string
	Google                     GoogleAuthConfig
}

type GoogleAuthConfig struct {
	Enabled               bool
	ClientIDs             []string
	AutoRegister          bool
	AutoLinkVerifiedEmail bool
	DefaultRole           string
	DefaultStatus         string
}

type SeedAdminConfig struct {
	Name     string
	Username string
	Email    string
	Password string
	WhatsApp string
	Role     string
}

type HTTPConfig struct {
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	IdleTimeoutSeconds  int
}

type MultiTenantConfig struct {
	PlatformOrganizationID   string
	PlatformOrganizationSlug string
	PlatformOrganizationName string
	PlatformPrimaryDomain    string
	ReservedSubdomains       []string
	TrustedProxyCIDRs        []string
	TrustForwardedHost       bool
	DefaultDataPlacement     string
}

type MailConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	Secure   bool
	TLS      bool
}

type NotificationConfig struct {
	DefaultLocale         string
	ProviderMode          string
	MaxAttempts           int
	WorkerIntervalSeconds int
	WorkerBatchSize       int
}

type MikrotikConfig struct {
	Host     string
	Username string
	Password string
}

type XenditConfig struct {
	BaseURL      string
	APIKey       string
	WebhookToken string
}

type WhatsAppConfig struct {
	Provider       string
	URL            string
	APIKey         string
	Session        string
	Sender         string
	CallbackURL    string
	TimeoutSeconds int
	TelegramBot    string
	ForumChatID    string
	TopikNews      string
}

type DiscordConfig struct {
	Provider         string
	Enabled          bool
	WebhookURL       string
	BotToken         string
	DefaultChannelID string
	Username         string
	AvatarURL        string
}

type SecurityConfig struct {
	EncryptionKey string
}

func (c AppConfig) Address() string {
	if c.Host == "" {
		return fmt.Sprintf(":%d", c.Port)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
