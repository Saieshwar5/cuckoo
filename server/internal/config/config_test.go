package config

import (
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// isolate clears every setting so a test sees only what it sets, regardless of
// what the developer happens to have exported in their shell.
func isolate(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CUCKOO_ENV", "CUCKOO_HTTP_ADDR", "CUCKOO_HUB_DOMAIN",
		"CUCKOO_DATABASE_URL", "CUCKOO_REDIS_URL",
		"CUCKOO_LOG_LEVEL", "CUCKOO_SHUTDOWN_TIMEOUT",
		"CUCKOO_RETENTION_DAYS", "CUCKOO_USER_MEDIA_BUDGET_MB", "CUCKOO_AGENT_MEDIA_BUDGET_MB",
		"CUCKOO_RETENTION_DRY_RUN", "CUCKOO_SIGNING_KEY",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with no settings returned an error: %v", err)
	}

	if cfg.Env != EnvDev {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDev)
	}
	if !cfg.IsDev() {
		t.Error("IsDev() = false for the default environment")
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 15s", cfg.ShutdownTimeout)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_ENV", "test")
	t.Setenv("CUCKOO_HTTP_ADDR", "127.0.0.1:9999")
	t.Setenv("CUCKOO_HUB_DOMAIN", "hub.example.org")
	t.Setenv("CUCKOO_LOG_LEVEL", "debug")
	t.Setenv("CUCKOO_SHUTDOWN_TIMEOUT", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}

	if cfg.Env != EnvTest {
		t.Errorf("Env = %q, want test", cfg.Env)
	}
	if cfg.IsDev() {
		t.Error("IsDev() = true for CUCKOO_ENV=test")
	}
	if cfg.HTTPAddr != "127.0.0.1:9999" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.HubDomain != "hub.example.org" {
		t.Errorf("HubDomain = %q", cfg.HubDomain)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
}

func TestLoadWhitespaceOnlyValueFallsBackToDefault(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_HTTP_ADDR", "   ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want the default", cfg.HTTPAddr)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]struct{ key, value string }{
		"unknown environment": {"CUCKOO_ENV", "staging"},
		"bad duration":        {"CUCKOO_SHUTDOWN_TIMEOUT", "soon"},
		"negative duration":   {"CUCKOO_SHUTDOWN_TIMEOUT", "-5s"},
		"bad log level":       {"CUCKOO_LOG_LEVEL", "chatty"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted %s=%q", tc.key, tc.value)
			} else if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error does not name the offending setting %s: %v", tc.key, err)
			}
		})
	}
}

// A misconfigured deployment should be fixable in one pass, not one restart per
// mistake.
func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_ENV", "staging")
	t.Setenv("CUCKOO_LOG_LEVEL", "chatty")
	t.Setenv("CUCKOO_SHUTDOWN_TIMEOUT", "soon")

	_, err := Load()
	if err == nil {
		t.Fatal("Load accepted three invalid settings")
	}

	for _, key := range []string{"CUCKOO_ENV", "CUCKOO_LOG_LEVEL", "CUCKOO_SHUTDOWN_TIMEOUT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error omits %s; all problems must be reported together:\n%v", key, err)
		}
	}
}

// Shipping a live hub pointed at a throwaway local database, or announcing
// itself as cuckoo.local, must be impossible by accident.
func TestProductionRejectsDevelopmentDefaults(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_ENV", "prod")

	_, err := Load()
	if err == nil {
		t.Fatal("production accepted development defaults")
	}
	for _, key := range []string{"CUCKOO_HUB_DOMAIN", "CUCKOO_DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("production error omits %s:\n%v", key, err)
		}
	}
}

func TestProductionAcceptsExplicitSettings(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_ENV", "prod")
	t.Setenv("CUCKOO_SIGNING_KEY", strings.Repeat("cd", 32))
	t.Setenv("CUCKOO_HUB_DOMAIN", "cuckoo.example")
	t.Setenv("CUCKOO_DATABASE_URL", "postgres://user:pass@db.internal:5432/cuckoo")
	t.Setenv("CUCKOO_MAIL", "console")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("production rejected explicit settings: %v", err)
	}
	if cfg.IsDev() {
		t.Error("IsDev() = true for CUCKOO_ENV=prod")
	}
}

func TestRetentionAndSigningDefaults(t *testing.T) {
	isolate(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Retention.MessageAge != 90*24*time.Hour {
		t.Errorf("MessageAge = %v, want 90 days", cfg.Retention.MessageAge)
	}
	if cfg.Retention.UserMediaBudget != 100<<20 || cfg.Retention.AgentMediaBudget != 1024<<20 {
		t.Errorf("budgets = %d, %d", cfg.Retention.UserMediaBudget, cfg.Retention.AgentMediaBudget)
	}
	if cfg.Retention.DryRun {
		t.Error("dry run is on by default")
	}
	if !cfg.DevSigningKey || len(cfg.SigningKey) != 32 {
		t.Errorf("dev signing key = %v (%d bytes)", cfg.DevSigningKey, len(cfg.SigningKey))
	}
}

func TestRetentionAndSigningReadEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv("CUCKOO_RETENTION_DAYS", "0")
	t.Setenv("CUCKOO_USER_MEDIA_BUDGET_MB", "5")
	t.Setenv("CUCKOO_RETENTION_DRY_RUN", "true")
	t.Setenv("CUCKOO_SIGNING_KEY", strings.Repeat("ab", 32))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Retention.MessageAge != 0 || cfg.Retention.UserMediaBudget != 5<<20 || !cfg.Retention.DryRun {
		t.Errorf("retention = %+v", cfg.Retention)
	}
	if cfg.DevSigningKey || cfg.SigningKey[0] != 0xab {
		t.Errorf("signing key not read: dev=%v key=%x", cfg.DevSigningKey, cfg.SigningKey[:2])
	}
}

func TestProductionNeedsARealSigningKey(t *testing.T) {
	prod := func(t *testing.T) {
		t.Helper()
		isolate(t)
		t.Setenv("CUCKOO_ENV", "prod")
		t.Setenv("CUCKOO_MAIL", "console")
		t.Setenv("CUCKOO_HUB_DOMAIN", "hub.example.org")
		t.Setenv("CUCKOO_DATABASE_URL", "postgres://real")
	}
	prod(t)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CUCKOO_SIGNING_KEY") {
		t.Errorf("production started without a signing key: %v", err)
	}
	prod(t)
	t.Setenv("CUCKOO_SIGNING_KEY", hex.EncodeToString(devSigningKey))
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "development key") {
		t.Errorf("production accepted the development key: %v", err)
	}
	prod(t)
	t.Setenv("CUCKOO_SIGNING_KEY", "nothex")
	if _, err := Load(); err == nil {
		t.Error("a malformed key was accepted")
	}
	prod(t)
	t.Setenv("CUCKOO_SIGNING_KEY", strings.Repeat("cd", 32))
	if _, err := Load(); err != nil {
		t.Errorf("production refused a real key: %v", err)
	}
}
