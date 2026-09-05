// Package config загружает и проверяет конфигурацию приложения.
//
// Единственный источник настроек — переменные окружения. Файл .env читается
// только в development, чтобы на боевом стенде нельзя было случайно поднять
// сервис с локальными секретами.
//
// Приложение падает при старте, если обязательная настройка отсутствует или
// некорректна. Это осознанный выбор: сервис, запущенный с пустым JWT_SECRET,
// опаснее, чем сервис, который не запустился.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Env описывает окружение выполнения.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

func (e Env) IsProduction() bool  { return e == EnvProduction }
func (e Env) IsDevelopment() bool { return e == EnvDevelopment }

// Config — полная конфигурация сервиса.
type Config struct {
	App       App
	HTTP      HTTP
	Postgres  Postgres
	Redis     Redis
	Auth      Auth
	RateLimit RateLimit
	Limits    Limits
	Storage   Storage
	Mailer    Mailer
	SMS       SMS
	Captcha   Captcha
	Social    Social
}

type App struct {
	Env       Env
	Name      string
	PublicURL string
	LogLevel  string
	LogFormat string
}

type HTTP struct {
	Host string
	Port int

	// TrustedProxies — сети, из которых разрешено доверять X-Forwarded-For.
	// Если список пуст, реальный IP всегда берётся из TCP-соединения.
	// Без этого ограничения любой клиент подделал бы заголовок и обошёл
	// ограничение частоты запросов.
	TrustedProxies []*net.IPNet

	AllowedOrigins []string

	// Таймауты сервера. Значения не настраиваются через окружение намеренно:
	// это часть защиты от медленных клиентов, а не тюнинг производительности.
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

func (h HTTP) Addr() string { return net.JoinHostPort(h.Host, strconv.Itoa(h.Port)) }

type Postgres struct {
	Host             string
	Port             int
	Database         string
	User             string
	Password         string
	SSLMode          string
	MaxConns         int32
	MinConns         int32
	StatementTimeout time.Duration

	// MigratorUser и MigratorPassword используются только командой миграций.
	// У роли приложения нет прав на изменение структуры базы.
	MigratorUser     string
	MigratorPassword string
}

// DSN собирает строку подключения для роли приложения.
func (p Postgres) DSN() string {
	return p.dsnFor(p.User, p.Password)
}

// MigratorDSN собирает строку подключения для роли миграций.
func (p Postgres) MigratorDSN() string {
	user, pass := p.MigratorUser, p.MigratorPassword
	if user == "" {
		user, pass = p.User, p.Password
	}
	return p.dsnFor(user, pass)
}

func (p Postgres) dsnFor(user, password string) string {
	// URL собирается через net/url, а не через конкатенацию: пароль со
	// служебными символами (@, /, ?) иначе ломает строку подключения.
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:     "/" + p.Database,
		RawQuery: url.Values{"sslmode": []string{p.SSLMode}}.Encode(),
	}
	return dsn.String()
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

type Auth struct {
	JWTSecret       []byte
	JWTIssuer       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// PIIKey — ключ AES-256-GCM для шифрования паспортных данных и адресов.
	PIIKey []byte

	Argon2MemoryKiB   uint32
	Argon2Iterations  uint32
	Argon2Parallelism uint8

	// MaxFailedLogins — после какого числа неудач аккаунт временно блокируется.
	MaxFailedLogins   int
	LockoutBase       time.Duration
	LockoutMax        time.Duration
	CaptchaAfterFails int

	VerificationCodeTTL     time.Duration
	VerificationMaxAttempts int
}

type RateLimit struct {
	Enabled bool

	GlobalPerMinute    int
	LoginPerMinute     int
	RegisterPerHour    int
	VerifyCodePer10Min int
	SearchPerMinute    int
	UploadPerHour      int

	AutobanEnabled  bool
	SuspiciousLimit int
	AutobanWindow   time.Duration
	AutobanBase     time.Duration
	AutobanMax      time.Duration
}

type Limits struct {
	MaxRequestBodyBytes int64
	MaxUploadBytes      int64
	MaxImagePixels      int64
	MaxPhotosPerCar     int
	MaxPageSize         int
	DefaultPageSize     int
	HeavyQueryParallel  int
}

type Storage struct {
	Driver        string
	LocalPath     string
	PublicBaseURL string
}

type Mailer struct {
	Driver   string
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

type SMS struct {
	Driver string
	APIKey string
	Sender string
}

type Captcha struct {
	Driver  string
	Secret  string
	SiteKey string
}

func (c Captcha) Enabled() bool { return c.Driver != "" && c.Driver != "disabled" }

// Social — разовые приложения площадки для OAuth дилеров.
//
// Пустые client id не роняют сервис: кнопки Instagram/YouTube в кабинете
// остаются серыми, пока приложения не заведены.
type Social struct {
	VKAppID            string
	VKAppSecret        string
	MetaAppID          string
	MetaAppSecret      string
	GoogleClientID     string
	GoogleClientSecret string
	OAuthRedirectURI   string
}

func (s Social) VKReady() bool {
	return s.VKAppID != "" && s.VKAppSecret != ""
}

func (s Social) MetaReady() bool {
	return s.MetaAppID != "" && s.MetaAppSecret != ""
}

func (s Social) GoogleReady() bool {
	return s.GoogleClientID != "" && s.GoogleClientSecret != ""
}

// Load читает конфигурацию из окружения.
//
// В development дополнительно подхватывается файл .env из корня репозитория,
// найденный поиском вверх по дереву каталогов, — так `go run ./cmd/api`
// работает из любой директории.
func Load() (*Config, error) {
	if env := os.Getenv("APP_ENV"); env == "" || env == string(EnvDevelopment) {
		if path, ok := findDotEnv(); ok {
			// Существующие переменные окружения имеют приоритет над файлом.
			_ = godotenv.Load(path)
		}
	}

	var errs []error
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	appEnv := Env(stringOr("APP_ENV", string(EnvDevelopment)))
	switch appEnv {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		collect(fmt.Errorf("APP_ENV: недопустимое значение %q", appEnv))
	}

	cfg := &Config{
		App: App{
			Env:       appEnv,
			Name:      stringOr("APP_NAME", "Auto Import CRM"),
			PublicURL: strings.TrimRight(stringOr("APP_PUBLIC_URL", "http://localhost:5173"), "/"),
			LogLevel:  stringOr("LOG_LEVEL", "info"),
			LogFormat: stringOr("LOG_FORMAT", "json"),
		},
		HTTP: HTTP{
			Host:           stringOr("HTTP_HOST", "0.0.0.0"),
			AllowedOrigins: splitList(stringOr("CORS_ALLOWED_ORIGINS", "")),

			// Пять секунд на передачу заголовков достаточно любому живому
			// клиенту и недостаточно slowloris-соединению.
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			ShutdownTimeout:   20 * time.Second,
			MaxHeaderBytes:    16 << 10,
		},
		Postgres: Postgres{
			Host:             stringOr("POSTGRES_HOST", "localhost"),
			Database:         stringOr("POSTGRES_DB", "autoimport"),
			User:             stringOr("POSTGRES_USER", "autoimport_app"),
			Password:         os.Getenv("POSTGRES_PASSWORD"),
			SSLMode:          stringOr("POSTGRES_SSLMODE", "disable"),
			MigratorUser:     os.Getenv("POSTGRES_MIGRATOR_USER"),
			MigratorPassword: os.Getenv("POSTGRES_MIGRATOR_PASSWORD"),
		},
		Redis: Redis{
			Addr:     stringOr("REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Auth: Auth{
			JWTIssuer: stringOr("JWT_ISSUER", "autoimport-crm"),

			MaxFailedLogins:         5,
			LockoutBase:             1 * time.Minute,
			LockoutMax:              1 * time.Hour,
			CaptchaAfterFails:       3,
			VerificationCodeTTL:     15 * time.Minute,
			VerificationMaxAttempts: 5,
		},
		Limits: Limits{
			MaxPhotosPerCar:    30,
			MaxPageSize:        100,
			DefaultPageSize:    24,
			HeavyQueryParallel: 4,
		},
		Storage: Storage{
			Driver:        stringOr("STORAGE_DRIVER", "local"),
			LocalPath:     stringOr("STORAGE_LOCAL_PATH", "./storage/uploads"),
			PublicBaseURL: strings.TrimRight(stringOr("STORAGE_PUBLIC_BASE_URL", "http://localhost:8080/files"), "/"),
		},
		Mailer: Mailer{
			Driver:   stringOr("MAILER_DRIVER", "log"),
			Host:     stringOr("SMTP_HOST", "localhost"),
			User:     os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     stringOr("SMTP_FROM", "Auto Import CRM <noreply@localhost>"),
		},
		SMS: SMS{
			Driver: stringOr("SMS_DRIVER", "log"),
			APIKey: os.Getenv("SMS_API_KEY"),
			Sender: stringOr("SMS_SENDER", "AUTOIMPORT"),
		},
		Captcha: Captcha{
			Driver:  stringOr("CAPTCHA_DRIVER", "disabled"),
			Secret:  os.Getenv("CAPTCHA_SECRET"),
			SiteKey: os.Getenv("CAPTCHA_SITE_KEY"),
		},
		Social: Social{
			VKAppID:            os.Getenv("VK_APP_ID"),
			VKAppSecret:        os.Getenv("VK_APP_SECRET"),
			MetaAppID:          os.Getenv("META_APP_ID"),
			MetaAppSecret:      os.Getenv("META_APP_SECRET"),
			GoogleClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
			GoogleClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		},
	}

	var err error

	cfg.HTTP.Port, err = intOr("HTTP_PORT", 8080)
	collect(err)
	cfg.HTTP.TrustedProxies, err = parseCIDRList(os.Getenv("TRUSTED_PROXY_CIDRS"))
	collect(err)

	cfg.Postgres.Port, err = intOr("POSTGRES_PORT", 5432)
	collect(err)
	maxConns, err := intOr("POSTGRES_MAX_CONNS", 20)
	collect(err)
	cfg.Postgres.MaxConns = int32(maxConns)
	minConns, err := intOr("POSTGRES_MIN_CONNS", 2)
	collect(err)
	cfg.Postgres.MinConns = int32(minConns)
	cfg.Postgres.StatementTimeout, err = durationOr("POSTGRES_STATEMENT_TIMEOUT", 15*time.Second)
	collect(err)

	cfg.Redis.DB, err = intOr("REDIS_DB", 0)
	collect(err)

	cfg.Auth.AccessTokenTTL, err = durationOr("ACCESS_TOKEN_TTL", 15*time.Minute)
	collect(err)
	cfg.Auth.RefreshTokenTTL, err = durationOr("REFRESH_TOKEN_TTL", 720*time.Hour)
	collect(err)

	argonMem, err := intOr("ARGON2_MEMORY_KIB", 65536)
	collect(err)
	cfg.Auth.Argon2MemoryKiB = uint32(argonMem)
	argonIter, err := intOr("ARGON2_ITERATIONS", 3)
	collect(err)
	cfg.Auth.Argon2Iterations = uint32(argonIter)
	argonPar, err := intOr("ARGON2_PARALLELISM", 2)
	collect(err)
	cfg.Auth.Argon2Parallelism = uint8(argonPar)

	cfg.RateLimit.Enabled = boolOr("RATELIMIT_ENABLED", true)
	cfg.RateLimit.GlobalPerMinute, err = intOr("RATELIMIT_GLOBAL_PER_MINUTE", 600)
	collect(err)
	cfg.RateLimit.LoginPerMinute, err = intOr("RATELIMIT_LOGIN_PER_MINUTE", 5)
	collect(err)
	cfg.RateLimit.RegisterPerHour, err = intOr("RATELIMIT_REGISTER_PER_HOUR", 3)
	collect(err)
	cfg.RateLimit.VerifyCodePer10Min, err = intOr("RATELIMIT_VERIFY_CODE_PER_10MIN", 3)
	collect(err)
	cfg.RateLimit.SearchPerMinute, err = intOr("RATELIMIT_SEARCH_PER_MINUTE", 60)
	collect(err)
	cfg.RateLimit.UploadPerHour, err = intOr("RATELIMIT_UPLOAD_PER_HOUR", 20)
	collect(err)
	cfg.RateLimit.AutobanEnabled = boolOr("AUTOBAN_ENABLED", true)
	cfg.RateLimit.SuspiciousLimit, err = intOr("AUTOBAN_SUSPICIOUS_THRESHOLD", 25)
	collect(err)
	cfg.RateLimit.AutobanWindow, err = durationOr("AUTOBAN_WINDOW", 5*time.Minute)
	collect(err)
	cfg.RateLimit.AutobanBase, err = durationOr("AUTOBAN_BASE_DURATION", 5*time.Minute)
	collect(err)
	cfg.RateLimit.AutobanMax, err = durationOr("AUTOBAN_MAX_DURATION", 24*time.Hour)
	collect(err)

	cfg.Limits.MaxRequestBodyBytes, err = int64Or("MAX_REQUEST_BODY_BYTES", 128<<10)
	collect(err)
	cfg.Limits.MaxUploadBytes, err = int64Or("MAX_UPLOAD_BYTES", 12<<20)
	collect(err)
	cfg.Limits.MaxImagePixels, err = int64Or("MAX_UPLOAD_IMAGE_PIXELS", 40_000_000)
	collect(err)

	cfg.Mailer.Port, err = intOr("SMTP_PORT", 1025)
	collect(err)

	cfg.Social.OAuthRedirectURI = strings.TrimRight(stringOr("OAUTH_REDIRECT_URI", ""), "/")
	if cfg.Social.OAuthRedirectURI == "" {
		cfg.Social.OAuthRedirectURI = cfg.App.PublicURL + "/api/v1/integrations/oauth/callback"
	}

	cfg.Auth.JWTSecret, err = requireSecret("JWT_SECRET", 32)
	collect(err)
	cfg.Auth.PIIKey, err = requireKey32("PII_ENCRYPTION_KEY")
	collect(err)

	if cfg.Postgres.Password == "" {
		collect(errors.New("POSTGRES_PASSWORD: обязательная переменная не задана"))
	}
	if cfg.Redis.Password == "" && cfg.App.Env.IsProduction() {
		collect(errors.New("REDIS_PASSWORD: в production Redis обязан требовать пароль"))
	}
	if cfg.App.Env.IsProduction() {
		if cfg.Postgres.SSLMode == "disable" {
			collect(errors.New("POSTGRES_SSLMODE: в production запрещено значение disable"))
		}
		if cfg.App.LogFormat != "json" {
			collect(errors.New("LOG_FORMAT: в production требуется json"))
		}
		if len(cfg.HTTP.AllowedOrigins) == 0 {
			collect(errors.New("CORS_ALLOWED_ORIGINS: в production список обязателен"))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("конфигурация некорректна: %w", errors.Join(errs...))
	}

	// На Windows `localhost` резолвится в ::1, а проброс портов WSL2 на IPv6
	// часто молчит. Postgres при этом живёт внутри Ubuntu — берём её IPv4.
	applyWindowsDevHosts(cfg)

	return cfg, nil
}

func applyWindowsDevHosts(cfg *Config) {
	if cfg == nil || !cfg.App.Env.IsDevelopment() || runtime.GOOS != "windows" {
		return
	}

	if cfg.Postgres.Host == "localhost" {
		cfg.Postgres.Host = "127.0.0.1"
	}
	if host, port, err := net.SplitHostPort(cfg.Redis.Addr); err == nil && host == "localhost" {
		cfg.Redis.Addr = net.JoinHostPort("127.0.0.1", port)
	}
}

// --- Вспомогательные функции чтения окружения ------------------------------

func stringOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return strings.Trim(v, `"`)
	}
	return fallback
}

func intOr(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback, fmt.Errorf("%s: ожидалось целое число, получено %q", key, raw)
	}
	return v, nil
}

func int64Or(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback, fmt.Errorf("%s: ожидалось целое число, получено %q", key, raw)
	}
	return v, nil
}

func boolOr(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func durationOr(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback, fmt.Errorf("%s: ожидалась длительность вида 15m, получено %q", key, raw)
	}
	return d, nil
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseCIDRList(raw string) ([]*net.IPNet, error) {
	items := splitList(raw)
	out := make([]*net.IPNet, 0, len(items))
	for _, item := range items {
		_, network, err := net.ParseCIDR(item)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS: %q не является подсетью вида 10.0.0.0/8", item)
		}
		out = append(out, network)
	}
	return out, nil
}

// requireSecret требует секрет минимальной длины. Короткий ключ подписи
// делает подделку токена вопросом вычислительного времени.
func requireSecret(key string, minLen int) ([]byte, error) {
	v := strings.Trim(strings.TrimSpace(os.Getenv(key)), `"`)
	if v == "" {
		return nil, fmt.Errorf("%s: обязательная переменная не задана", key)
	}
	if len(v) < minLen {
		return nil, fmt.Errorf("%s: требуется не менее %d символов, получено %d", key, minLen, len(v))
	}
	return []byte(v), nil
}

// requireKey32 принимает ключ шифрования как 32 произвольных байта.
// Значение можно задать и в base64 — оно будет распознано автоматически.
func requireKey32(key string) ([]byte, error) {
	v := strings.Trim(strings.TrimSpace(os.Getenv(key)), `"`)
	if v == "" {
		return nil, fmt.Errorf("%s: обязательная переменная не задана", key)
	}
	if decoded, err := decodeBase64(v); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(v) == 32 {
		return []byte(v), nil
	}
	return nil, fmt.Errorf(
		"%s: требуется 32 байта — либо ровно 32 символа, либо base64 от 32 байт (получено %d символов)",
		key, len(v),
	)
}

// decodeBase64 принимает как стандартный, так и URL-безопасный алфавит.
func decodeBase64(v string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(v)
}

// findDotEnv ищет .env, поднимаясь вверх от рабочего каталога.
func findDotEnv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for range 6 {
		candidate := filepath.Join(dir, ".env")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
