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
	for _, key := range []string{
		"CUCKOO_MAIL", "CUCKOO_SMTP_HOST", "CUCKOO_SMTP_PORT",
		"CUCKOO_SMTP_USERNAME", "CUCKOO_SMTP_PASSWORD", "CUCKOO_SMTP_FROM", "CUCKOO_SMTP_FROM_NAME",
	} {
		t.Setenv(key, "")
	}
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

// The SMTP mode needs a provider to send through, and says which part is
// missing rather than starting and failing on the first sign-in.
func TestSMTPModeNeedsAProvider(t *testing.T) {
	prodEnv(t)
	t.Setenv("CUCKOO_MAIL", "smtp")
	_, err := config.Load()
	if err == nil {
		t.Fatal("smtp mode started with no provider")
	}
	for _, key := range []string{"CUCKOO_SMTP_HOST", "CUCKOO_SMTP_FROM"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("the refusal does not name %s:\n%v", key, err)
		}
	}

	prodEnv(t)
	t.Setenv("CUCKOO_MAIL", "smtp")
	t.Setenv("CUCKOO_SMTP_HOST", "email-smtp.ap-south-1.amazonaws.com")
	t.Setenv("CUCKOO_SMTP_FROM", "hello@cuckoo.example")
	t.Setenv("CUCKOO_SMTP_USERNAME", "u")
	t.Setenv("CUCKOO_SMTP_PASSWORD", "p")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("smtp mode with a provider: %v", err)
	}
	if cfg.Mail != config.MailSMTP || cfg.SMTP.Port != 587 || cfg.SMTP.FromName != "Cuckoo" {
		t.Errorf("smtp settings = %+v", cfg.SMTP)
	}
}
