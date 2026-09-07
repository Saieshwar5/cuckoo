package config_test

import (
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/config"
)

func prodEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CUCKOO_ENV", "prod")
	t.Setenv("CUCKOO_HUB_DOMAIN", "cuckoo.example")
	t.Setenv("CUCKOO_DATABASE_URL", "postgres://u:p@db.example/cuckoo")
	t.Setenv("CUCKOO_SIGNING_KEY", strings.Repeat("cd", 32))
}

// Console mail is a development convenience that must be chosen on purpose
// anywhere else: a production hub with no idea where codes go must not start.
func TestMailModeDefaultsOnlyInDevelopment(t *testing.T) {
	t.Setenv("CUCKOO_ENV", "dev")
	t.Setenv("CUCKOO_MAIL", "")
	cfg, err := config.Load()
	if err != nil || cfg.Mail != config.MailConsole {
		t.Fatalf("dev: mail = %q, %v; want console by default", cfg.Mail, err)
	}

	prodEnv(t)
	t.Setenv("CUCKOO_MAIL", "")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "CUCKOO_MAIL") {
		t.Errorf("prod without CUCKOO_MAIL: %v, want a refusal naming it", err)
	}

	t.Setenv("CUCKOO_MAIL", "console")
	if cfg, err := config.Load(); err != nil || cfg.Mail != config.MailConsole {
		t.Errorf("prod with console set deliberately: %q, %v; want to start", cfg.Mail, err)
	}

	t.Setenv("CUCKOO_MAIL", "pigeon")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "CUCKOO_MAIL") {
		t.Errorf("unknown mail mode: %v, want a refusal", err)
	}
}
