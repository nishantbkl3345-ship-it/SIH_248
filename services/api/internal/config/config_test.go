package config

import (
	"strings"
	"testing"
	"time"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
		"JWT_SECRET":   strings.Repeat("s", 32),
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(validEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != EnvDevelopment || cfg.HTTPAddr != ":8080" || cfg.JWTTTL != 8*time.Hour {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if !cfg.DBAutoMigrate || cfg.CookieSecure || !cfg.AllowRegistration {
		t.Errorf("unexpected development defaults: %+v", cfg)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("unexpected CORS origins: %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadProductionDefaults(t *testing.T) {
	kv := validEnv()
	kv["APP_ENV"] = "production"
	kv["CORS_ALLOWED_ORIGINS"] = "https://a.example/, https://b.example"
	cfg, err := Load(env(kv))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBAutoMigrate || !cfg.CookieSecure {
		t.Errorf("unexpected production defaults: %+v", cfg)
	}
	if got := strings.Join(cfg.CORSAllowedOrigins, ","); got != "https://a.example,https://b.example" {
		t.Errorf("CORS origins = %q", got)
	}
}

func TestLoadReportsAllProblems(t *testing.T) {
	_, err := Load(env(map[string]string{
		"APP_ENV":               "staging",
		"JWT_SECRET":            "short",
		"JWT_TTL":               "soon",
		"COOKIE_SECURE":         "maybe",
		"LOG_LEVEL":             "loud",
		"BOOTSTRAP_ADMIN_EMAIL": "admin@example.test",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{
		"DATABASE_URL is required",
		"APP_ENV must be one of",
		"JWT_SECRET must be at least",
		"JWT_TTL must be a positive duration",
		"COOKIE_SECURE must be a boolean",
		"LOG_LEVEL must be one of",
		"must be set together",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q:\n%v", want, err)
		}
	}
}

func TestLoadRejectsNonPostgresURL(t *testing.T) {
	kv := validEnv()
	kv["DATABASE_URL"] = "mysql://localhost/db"
	if _, err := Load(env(kv)); err == nil || !strings.Contains(err.Error(), "DATABASE_URL must start with") {
		t.Fatalf("expected DATABASE_URL scheme error, got %v", err)
	}
}
