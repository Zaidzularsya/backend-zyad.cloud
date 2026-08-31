package config

import (
	"fmt"
	"sync"
	"time"
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
	Doku         DokuConfig
	WhatsApp     WhatsAppConfig
	Discord      DiscordConfig
	Security     SecurityConfig
	Storage      StorageConfig
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
		Doku:         LoadDoku(),
		WhatsApp:     LoadWhatsApp(),
		Discord:      LoadDiscord(),
		Security:     LoadSecurity(),
		Storage:      LoadStorage(),
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

// DokuConfig holds DOKU Checkout API credentials. ClientID + SecretKey drive
// the per-request HMAC signature scheme (both outgoing checkout calls and
// incoming notification verification). APIKey is DOKU's newer dashboard-issued
// key (doku_key_...); it is not used by the Checkout API but kept configured
// for other DOKU surfaces.
type DokuConfig struct {
	BaseURL   string
	ClientID  string
	SecretKey string
	APIKey    string
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

// StorageConfig mengatur penyimpanan file (media upload).
//
// Driver memilih provider aktif:
//   - "local": simpan di disk di bawah LocalPath (default). Di VPS taruh di
//     shared/ agar tahan deploy.
//   - "s3": simpan di object storage S3-compatible (MinIO self-hosted).
//     Field S3* wajib diisi saat Driver == "s3".
type StorageConfig struct {
	Driver    string
	LocalPath string

	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3UsePathStyle  bool
	S3PresignExpiry time.Duration
}

func (c AppConfig) Address() string {
	if c.Host == "" {
		return fmt.Sprintf(":%d", c.Port)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
