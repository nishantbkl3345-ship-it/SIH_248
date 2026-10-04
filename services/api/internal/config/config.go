// Package config loads and validates process configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// minJWTSecretLen is the minimum accepted HS256 key length in bytes.
const minJWTSecretLen = 32

type Config struct {
	Env      string
	HTTPAddr string
	LogLevel string

	DatabaseURL   string
	DBAutoMigrate bool

	JWTSecret string
	JWTIssuer string
	JWTTTL    time.Duration

	CookieSecure       bool
	CORSAllowedOrigins []string
	AllowRegistration  bool

	// Optional. When both are set, an ADMIN with these credentials is created
	// at startup if no user with that email exists.
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// Load reads configuration through getenv (os.Getenv in production) and
// reports every problem at once rather than failing on the first.
func Load(getenv func(string) string) (Config, error) {
	var errs []error

	str := func(key, fallback string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return fallback
	}
	required := func(key string) string {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
		return v
	}
	boolean := func(key string, fallback bool) bool {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return fallback
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a boolean, got %q", key, v))
			return fallback
		}
		return b
	}
	duration := func(key string, fallback time.Duration) time.Duration {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return fallback
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration such as 8h, got %q", key, v))
			return fallback
		}
		return d
	}

	cfg := Config{
		Env:                    str("APP_ENV", EnvDevelopment),
		HTTPAddr:               str("HTTP_ADDR", ":8080"),
		LogLevel:               strings.ToLower(str("LOG_LEVEL", "info")),
		DatabaseURL:            required("DATABASE_URL"),
		JWTSecret:              required("JWT_SECRET"),
		JWTIssuer:              str("JWT_ISSUER", "fogline"),
		JWTTTL:                 duration("JWT_TTL", 8*time.Hour),
		BootstrapAdminEmail:    str("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword: getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}

	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be one of development, test, production, got %q", cfg.Env))
	}

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error, got %q", cfg.LogLevel))
	}

	if cfg.DatabaseURL != "" &&
		!strings.HasPrefix(cfg.DatabaseURL, "postgres://") &&
		!strings.HasPrefix(cfg.DatabaseURL, "postgresql://") {
		errs = append(errs, errors.New("DATABASE_URL must start with postgres:// or postgresql://"))
	}

	if cfg.JWTSecret != "" && len(cfg.JWTSecret) < minJWTSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d characters", minJWTSecretLen))
	}

	cfg.DBAutoMigrate = boolean("DB_AUTO_MIGRATE", !cfg.IsProduction())
	cfg.CookieSecure = boolean("COOKIE_SECURE", cfg.IsProduction())
	cfg.AllowRegistration = boolean("ALLOW_REGISTRATION", true)

	for _, o := range strings.Split(str("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, strings.TrimRight(o, "/"))
		}
	}

	if (cfg.BootstrapAdminEmail == "") != (cfg.BootstrapAdminPassword == "") {
		errs = append(errs, errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD must be set together"))
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}
