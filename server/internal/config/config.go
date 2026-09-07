// Package config loads and validates the server's runtime configuration.
//
// Configuration comes from the environment only (twelve-factor): the same
// binary runs in dev, in test and in production, and the only thing that
// differs is the environment. Deploying is therefore "edit .env".
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
	// PublicURL is where people reach this hub from outside: the base of
	// the links inside QR codes. The laptop's address in development, the
	// hub's domain in production.
	PublicURL string

	DatabaseURL string
	RedisURL    string

	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	// WelcomeHandle names the agent every new account is given on arrival,
	// by its handle on this hub. Empty means nobody: a new account opens on
	// an empty list. The agent is created and run by the hub's operator
	// like any other; this only says which one is the greeter.
	WelcomeHandle string

	// MediaDir is the directory uploaded files are kept in. A folder on
	// this machine is the whole of storage today; the seam for a bucket is
	// in the blobs package, not here.
	MediaDir string

	// Mail is how sign-in codes are delivered. "console" prints them to the
	// log, which is right for development and for a private hub whose
	// operator is its only user; it is never assumed in production.
	Mail string

	// CORSOrigins are the browser origins allowed to call the API. Any
	// origin in development, none in production unless listed.
	CORSOrigins []string
}

// Mail modes.
const MailConsole = "console"

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
		MediaDir:        l.str("CUCKOO_MEDIA_DIR", "./.data/media"),
		WelcomeHandle:   l.str("CUCKOO_WELCOME_HANDLE", ""),
		Mail:            l.str("CUCKOO_MAIL", ""),
		CORSOrigins:     l.list("CUCKOO_CORS_ORIGINS"),
	}
	if len(cfg.CORSOrigins) == 0 && cfg.Env == EnvDev {
		cfg.CORSOrigins = []string{"*"}
	}
	cfg.PublicURL = strings.TrimRight(l.str("CUCKOO_PUBLIC_URL", ""), "/")
	if cfg.PublicURL == "" {
		if cfg.Env == EnvDev {
			cfg.PublicURL = "http://localhost" + portOf(cfg.HTTPAddr)
		} else {
			cfg.PublicURL = "https://" + cfg.HubDomain
		}
	}

	switch {
	case cfg.Mail == "" && cfg.Env == EnvProd:
		l.fail("CUCKOO_MAIL must be set when CUCKOO_ENV=prod: sign-in codes have to go somewhere. " +
			"The only mode today is console, which prints them to the log; set it deliberately or not at all")
	case cfg.Mail == "":
		cfg.Mail = MailConsole
	case cfg.Mail != MailConsole:
		l.fail("CUCKOO_MAIL must be console (got %q)", cfg.Mail)
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

// list reads a comma-separated value; empty means an empty list.
func (l *loader) list(key string) []string {
	var out []string
	for _, v := range strings.Split(l.str(key, ""), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
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

// portOf is the ":port" of a listen address, or nothing for port 80.
func portOf(addr string) string {
	i := strings.LastIndex(addr, ":")
	if i < 0 || addr[i:] == ":80" {
		return ""
	}
	return addr[i:]
}
