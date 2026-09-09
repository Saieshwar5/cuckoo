// Package config loads and validates the server's runtime configuration.
//
// Configuration comes from the environment only (twelve-factor): the same
// binary runs in dev, in test and in production, and the only thing that
// differs is the environment. Deploying is therefore "edit .env".
package config

import (
	"bytes"
	"strconv"

	"errors"
	"fmt"
	"github.com/Saieshwar5/cuckoo/server/internal/signing"
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

	// Blobs is where uploaded files are kept: BlobsDisk, a folder on this
	// machine, or BlobsS3, a bucket. Both use the same keys, so moving
	// between them is a copy and this setting, not a migration.
	Blobs string
	// MediaDir is the directory uploaded files are kept in when Blobs is
	// BlobsDisk.
	MediaDir string
	// S3 is the bucket uploaded files go to when Blobs is BlobsS3.
	S3 S3Settings

	// Mail is how sign-in codes are delivered. "console" prints them to the
	// log, which is right for development and for a private hub whose
	// operator is its only user; it is never assumed in production.
	Mail string
	// SMTP is where mail goes when Mail is "smtp". Empty otherwise.
	SMTP SMTPSettings

	// ExpoAccessToken authenticates the hub to Expo's push service. Optional:
	// Expo requires it only for accounts that have turned on enhanced
	// security, and notifications work without one until then. Empty is a hub
	// that still records device addresses and still decides who to notify —
	// the sending is simply refused at the far end.
	ExpoAccessToken string

	// Retention is what the hub keeps and for how long: messages for a
	// period, each uploader's files to a budget. See the retention package.
	Retention Retention

	// SigningKey is what the hub marks every finished message with, so a
	// copy kept elsewhere can be shown to be what was said. Required in
	// production; a fixed development key otherwise, and DevSigningKey says
	// when that is the case so it can be logged.
	SigningKey    []byte
	DevSigningKey bool

	// CORSOrigins are the browser origins allowed to call the API. Any
	// origin in development, none in production unless listed.
	CORSOrigins []string
}

// Mail modes.
const (
	// MailConsole prints codes to the log. Development, and a hub whose
	// only user reads its log.
	MailConsole = "console"
	// MailSMTP sends through a provider.
	MailSMTP = "smtp"
)

// Blob backends.
const (
	// BlobsDisk keeps uploaded files in a directory on this machine.
	BlobsDisk = "disk"
	// BlobsS3 keeps them in a bucket.
	BlobsS3 = "s3"
)

// S3Settings is the bucket uploaded files go to. There are no credentials
// here on purpose: the AWS SDK finds them the way every AWS tool does, which
// on the hub's machine is the instance's own role, so nothing secret has to
// live in .env for storage to work.
type S3Settings struct {
	Bucket string
	// Region may be empty, leaving the SDK to resolve it.
	Region string
}

// SMTPSettings is the provider the hub sends through.
type SMTPSettings struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

// Retention is the hub's window. Zero MessageAge keeps messages forever;
// a zero budget is no budget.
type Retention struct {
	MessageAge       time.Duration
	UserMediaBudget  int64
	AgentMediaBudget int64
	DryRun           bool
}

// devSigningKey signs messages on a development hub. It is public, which is
// the point: nothing signed with it proves anything, and production refuses
// to start with it.
var devSigningKey = bytes.Repeat([]byte{0x63, 0x75}, 16) // "cu" ×16, 32 bytes

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
		Blobs:           l.oneOf("CUCKOO_BLOBS", BlobsDisk, BlobsDisk, BlobsS3),
		MediaDir:        l.str("CUCKOO_MEDIA_DIR", "./.data/media"),
		WelcomeHandle:   l.str("CUCKOO_WELCOME_HANDLE", ""),
		Mail:            l.str("CUCKOO_MAIL", ""),
		CORSOrigins:     l.list("CUCKOO_CORS_ORIGINS"),
		Retention: Retention{
			MessageAge:       time.Duration(l.count("CUCKOO_RETENTION_DAYS", 90)) * 24 * time.Hour,
			UserMediaBudget:  l.count("CUCKOO_USER_MEDIA_BUDGET_MB", 100) << 20,
			AgentMediaBudget: l.count("CUCKOO_AGENT_MEDIA_BUDGET_MB", 1024) << 20,
			DryRun:           l.flag("CUCKOO_RETENTION_DRY_RUN", false),
		},
	}
	cfg.SigningKey, cfg.DevSigningKey = l.signingKey("CUCKOO_SIGNING_KEY", cfg.Env)
	cfg.ExpoAccessToken = l.str("CUCKOO_EXPO_ACCESS_TOKEN", "")
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

	if cfg.Blobs == BlobsS3 {
		cfg.S3 = S3Settings{
			Bucket: l.str("CUCKOO_S3_BUCKET", ""),
			Region: l.str("CUCKOO_S3_REGION", ""),
		}
		if cfg.S3.Bucket == "" {
			l.fail("CUCKOO_S3_BUCKET must be set when CUCKOO_BLOBS=s3")
		}
	}

	switch {
	case cfg.Mail == "" && cfg.Env == EnvProd:
		l.fail("CUCKOO_MAIL must be set when CUCKOO_ENV=prod: sign-in codes have to go somewhere. " +
			"Either smtp with a provider, or console, which prints them to the log — deliberately")
	case cfg.Mail == "":
		cfg.Mail = MailConsole
	case cfg.Mail == MailSMTP:
		cfg.SMTP = SMTPSettings{
			Host:     l.str("CUCKOO_SMTP_HOST", ""),
			Port:     int(l.count("CUCKOO_SMTP_PORT", 587)),
			Username: l.str("CUCKOO_SMTP_USERNAME", ""),
			Password: l.str("CUCKOO_SMTP_PASSWORD", ""),
			From:     l.str("CUCKOO_SMTP_FROM", ""),
			FromName: l.str("CUCKOO_SMTP_FROM_NAME", "Cuckoo"),
		}
		for key, value := range map[string]string{
			"CUCKOO_SMTP_HOST": cfg.SMTP.Host,
			"CUCKOO_SMTP_FROM": cfg.SMTP.From,
		} {
			if value == "" {
				l.fail("%s must be set when CUCKOO_MAIL=smtp", key)
			}
		}
	case cfg.Mail != MailConsole:
		l.fail("CUCKOO_MAIL must be console or smtp (got %q)", cfg.Mail)
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

// count reads a non-negative whole number.
func (l *loader) count(key string, def int64) int64 {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		l.fail("%s must be a whole number, 0 or more (got %q)", key, raw)
		return def
	}
	return n
}

// flag reads true or false.
func (l *loader) flag(key string, def bool) bool {
	raw := l.str(key, "")
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		l.fail("%s must be true or false (got %q)", key, raw)
		return def
	}
	return b
}

// signingKey reads the message signing key. Production must set one;
// anywhere else the development key stands in, and the second result says
// so.
func (l *loader) signingKey(key string, env Env) ([]byte, bool) {
	raw := l.str(key, "")
	if raw == "" {
		if env == EnvProd {
			l.fail("%s must be set when CUCKOO_ENV=prod: 64 hex characters, e.g. from `openssl rand -hex 32`", key)
		}
		return devSigningKey, true
	}
	parsed, err := signing.ParseKey(raw)
	if err != nil {
		l.fail("%s: %v", key, err)
		return devSigningKey, true
	}
	if bytes.Equal(parsed, devSigningKey) {
		if env == EnvProd {
			l.fail("%s is the development key, which is public", key)
		}
		return devSigningKey, true
	}
	return parsed, false
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
