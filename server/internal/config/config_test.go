package config

import (
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
