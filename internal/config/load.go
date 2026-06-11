package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func LoadApp() AppConfig {
	return AppConfig{
		Name:           getEnv("APP_NAME", "zyad.cloud"),
		Host:           getEnv("APP_HOST", "0.0.0.0"),
		URL:            getEnv("APP_URL", "http://localhost:3000"),
		FrontendURL:    getEnv("APP_FRONTEND_URL", "http://localhost:3001"),
		Secret:         getEnv("APP_SECRET", ""),
		Version:        getEnv("APP_VERSION", "dev"),
		Port:           getEnvInt("APP_PORT", 3000),
		OrganizationID: getEnv("APP_ORGANIZATION_ID", ""),
		Env:            getEnv("NODE_ENV", "development"),
	}
}

func LoadDatabase() DatabaseConfig {
	return DatabaseConfig{
		Type:     getEnv("DB_TYPE", "postgres"),
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		Username: getEnv("DB_USERNAME", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		Name:     getEnv("DB_NAME", "zyad_cloud"),
		Schema:   getEnv("DB_SCHEMA", "public"),
		SSLMode:  getEnv("DB_SSL", "disable"),
	}
}

func LoadTestDatabase() DatabaseConfig {
	loadEnvOnce.Do(func() {
		_ = loadDefaultDotEnv()
	})

	return DatabaseConfig{
		Type:     getEnv("TEST_DB_TYPE", "postgres"),
		Host:     getEnv("TEST_DB_HOST", "localhost"),
		Port:     getEnvInt("TEST_DB_PORT", 5432),
		Username: getEnv("TEST_DB_USERNAME", "postgres"),
		Password: getEnv("TEST_DB_PASSWORD", "postgres"),
		Name:     getEnv("TEST_DB_NAME", "zyad_cloud_test"),
		Schema:   getEnv("TEST_DB_SCHEMA", "public"),
		SSLMode:  getEnv("TEST_DB_SSL", "disable"),
	}
}

func LoadRedis() RedisConfig {
	return RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnvInt("REDIS_PORT", 6379),
		Password: getEnv("REDIS_PASSWORD", ""),
	}
}

func LoadAuth() AuthConfig {
	return AuthConfig{
		Secret:                     getEnv("JWT_SECRET", "dev-only-jwt-secret-change-me-32-bytes"),
		ExpiresIn:                  getEnv("JWT_EXPIRES_IN", "15m"),
		RefreshSecret:              getEnv("JWT_REFRESH_SECRET", "dev-only-refresh-secret-change-me-32-bytes"),
		RefreshExpiresIn:           getEnv("JWT_REFRESH_EXPIRES_IN", "7d"),
		RememberMeRefreshExpiresIn: getEnv("JWT_REMEMBER_ME_REFRESH_EXPIRES_IN", "30d"),
		ResetTokenExpiresIn:        getEnv("AUTH_RESET_TOKEN_EXPIRES_IN", "15m"),
		VerificationTokenExpiresIn: getEnv("AUTH_VERIFICATION_TOKEN_EXPIRES_IN", "24h"),
		OTPExpiresIn:               getEnv("AUTH_OTP_EXPIRES_IN", "5m"),
		OTPMaxAttempts:             getEnvInt("AUTH_OTP_MAX_ATTEMPTS", 5),
		PasswordMinLength:          getEnvInt("AUTH_PASSWORD_MIN_LENGTH", 8),
		CookieSecret:               getEnv("COOKIE_SECRET", ""),
		CookieExpiresIn:            getEnv("COOKIE_EXPIRES_IN", "24h"),
	}
}

func LoadSeedAdmin() SeedAdminConfig {
	return SeedAdminConfig{
		Name:     getEnv("SEED_ADMIN_NAME", ""),
		Username: getEnv("SEED_ADMIN_USERNAME", ""),
		Email:    getEnv("SEED_ADMIN_EMAIL", ""),
		Password: getEnv("SEED_ADMIN_PASSWORD", ""),
		WhatsApp: getEnv("SEED_ADMIN_WHATSAPP", ""),
		Role:     getEnv("SEED_ADMIN_ROLE", "super_admin"),
	}
}

func LoadHTTP() HTTPConfig {
	return HTTPConfig{
		ReadTimeoutSeconds:  getEnvInt("HTTP_READ_TIMEOUT_SECONDS", 15),
		WriteTimeoutSeconds: getEnvInt("HTTP_WRITE_TIMEOUT_SECONDS", 15),
		IdleTimeoutSeconds:  getEnvInt("HTTP_IDLE_TIMEOUT_SECONDS", 60),
	}
}

func LoadMail() MailConfig {
	return MailConfig{
		Host:     getEnv("MAIL_HOST", ""),
		Port:     getEnvInt("MAIL_PORT", 587),
		User:     getEnv("MAIL_USER", ""),
		Password: getEnv("MAIL_PASSWORD", ""),
		From:     getEnv("MAIL_FROM", ""),
		Secure:   getEnvBool("MAIL_SECURE", false),
		TLS:      getEnvBool("MAIL_TLS", true),
	}
}

func LoadNotification() NotificationConfig {
	return NotificationConfig{
		DefaultLocale: getEnv("NOTIFICATION_DEFAULT_LOCALE", "id-ID"),
		ProviderMode:  getEnv("NOTIFICATION_PROVIDER_MODE", "noop"),
		MaxAttempts:   getEnvInt("NOTIFICATION_MAX_ATTEMPTS", 3),
	}
}

func LoadMikrotik() MikrotikConfig {
	return MikrotikConfig{
		Host:     getEnv("MIKROTIK_HOST", ""),
		Username: getEnv("MIKROTIK_USERNAME", ""),
		Password: getEnv("MIKROTIK_PASSWORD", ""),
	}
}

func LoadXendit() XenditConfig {
	return XenditConfig{
		BaseURL:      getEnv("XENDIT_BASE_URL", ""),
		APIKey:       getEnv("XENDIT_API_KEY", ""),
		WebhookToken: getEnv("XENDIT_WEBHOOK_TOKEN", ""),
	}
}

func LoadWhatsApp() WhatsAppConfig {
	return WhatsAppConfig{
		Provider:       getEnv("WHATSAPP_PROVIDER", "noop"),
		URL:            getEnv("WHATSAPP_API_URL", ""),
		APIKey:         getEnv("WHATSAPP_API_KEY", ""),
		Session:        getEnv("WHATSAPP_API_SESSION", ""),
		Sender:         getEnv("WHATSAPP_SENDER", ""),
		CallbackURL:    getEnv("WHATSAPP_API_URL_CALLBACK", ""),
		TimeoutSeconds: getEnvInt("WHATSAPP_TIMEOUT_SECONDS", 15),
		TelegramBot:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		ForumChatID:    getEnv("LUNADESK_FORUM_CHATID", ""),
		TopikNews:      getEnv("TOPIK_NEWS", ""),
	}
}

func LoadDiscord() DiscordConfig {
	return DiscordConfig{
		Provider:         getEnv("DISCORD_PROVIDER", "noop"),
		Enabled:          getEnvBool("DISCORD_ENABLED", false),
		WebhookURL:       getEnv("DISCORD_WEBHOOK_URL", ""),
		BotToken:         getEnv("DISCORD_BOT_TOKEN", ""),
		DefaultChannelID: getEnv("DISCORD_DEFAULT_CHANNEL_ID", ""),
		Username:         getEnv("DISCORD_USERNAME", ""),
		AvatarURL:        getEnv("DISCORD_AVATAR_URL", ""),
	}
}

func LoadSecurity() SecurityConfig {
	return SecurityConfig{
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func loadDefaultDotEnv() error {
	path, err := findUp(".env")
	if err != nil {
		return err
	}
	return loadDotEnv(path)
}

func findUp(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func parseDotEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	line = strings.TrimPrefix(line, "export ")
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}

	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
		}
	}

	return key, value, true
}
