// Package config loads and validates the server's runtime configuration.
//
// Configuration comes from the environment only (twelve-factor): the same
// binary runs in dev, in production, and on a self-hosted hub, and the only
// thing that differs is the environment. Self-hosting is therefore "edit .env".
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Env is the deployment environment. It gates behaviour that must never be
// available in production, such as the development authentication bypass.
type Env string

const (
	EnvDev  Env = "dev"
	EnvTest Env = "test"
	EnvProd Env = "prod"
)

// Config is the fully validated configuration for one server process.
type Config struct {
	Env       Env
	HTTPAddr  string
	HubDomain string

	DatabaseURL string
	RedisURL    string

	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// IsDev reports whether development-only behaviour is permitted.
func (c Config) IsDev() bool { return c.Env == EnvDev }

// Load reads configuration from the environment.
//
// Every problem is reported at once rather than one per run, so a misconfigured
// deployment is fixed in a single pass.
func Load() (Config, error) {
	l := &loader{}

	cfg := Config{
		Env:             Env(l.oneOf("CUCKOO_ENV", string(EnvDev), string(EnvDev), string(EnvTest), string(EnvProd))),
		HTTPAddr:        l.str("CUCKOO_HTTP_ADDR", ":8080"),
		HubDomain:       l.str("CUCKOO_HUB_DOMAIN", "cuckoo.local"),
		DatabaseURL:     l.str("CUCKOO_DATABASE_URL", "postgres://cuckoo:cuckoo@localhost:5433/cuckoo?sslmode=disable"),
		RedisURL:        l.str("CUCKOO_REDIS_URL", "redis://localhost:6380/0"),
		LogLevel:        l.logLevel("CUCKOO_LOG_LEVEL", slog.LevelInfo),
		ShutdownTimeout: l.duration("CUCKOO_SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	if cfg.Env == EnvProd {
		l.requireNonDefault("CUCKOO_HUB_DOMAIN", cfg.HubDomain, "cuckoo.local")
		l.requireNonDefault("CUCKOO_DATABASE_URL", cfg.DatabaseURL,
			"postgres://cuckoo:cuckoo@localhost:5433/cuckoo?sslmode=disable")
	}

	if err := l.err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loader accumulates configuration errors so they can all be reported together.
type loader struct{ problems []string }

func (l *loader) fail(format string, args ...any) {
	l.problems = append(l.problems, fmt.Sprintf(format, args...))
}

func (l *loader) err() error {
	if len(l.problems) == 0 {
		return nil
	}
	return errors.New("invalid configuration:\n  - " + strings.Join(l.problems, "\n  - "))
}

func (l *loader) str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func (l *loader) oneOf(key, def string, allowed ...string) string {
	v := l.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.fail("%s must be one of %s (got %q)", key, strings.Join(allowed, ", "), v)
	return def
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		l.fail("%s must be a duration such as 15s (got %q)", key, raw)
		return def
	}
	if d <= 0 {
		l.fail("%s must be positive (got %q)", key, raw)
		return def
	}
	return d
}

func (l *loader) logLevel(key string, def slog.Level) slog.Level {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(raw)); err != nil {
		l.fail("%s must be debug, info, warn or error (got %q)", key, raw)
		return def
	}
	return lvl
}

// requireNonDefault rejects a development default that would be unsafe in
// production, such as pointing a live hub at a throwaway local database.
func (l *loader) requireNonDefault(key, value, devDefault string) {
	if value == devDefault {
		l.fail("%s must be set explicitly when CUCKOO_ENV=prod", key)
	}
}
