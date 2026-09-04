package agents

import (
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

func TestValidateHandle(t *testing.T) {
	valid := map[string]string{
		"sbi-support":           "sbi-support",
		"my_helper":             "my_helper",
		"abc":                   "abc",
		"a1":                    "", // too short
		"MyHelper":              "myhelper",
		"  spaced  ":            "spaced",
		"123":                   "123",
		"x-y_z9":                "x-y_z9",
		"-leading":              "",
		"_leading":              "",
		"has space":             "",
		"has.dot":               "",
		"ప్రియ":                 "",
		"":                      "",
		strings.Repeat("a", 32): strings.Repeat("a", 32),
		strings.Repeat("a", 33): "",
	}

	for input, want := range valid {
		t.Run(input, func(t *testing.T) {
			got, err := validateHandle(input)
			if want == "" {
				if err == nil {
					t.Fatalf("validateHandle(%q) accepted %q", input, got)
				}
				if d, _ := domain.AsError(err); d.Field != "handle" {
					t.Errorf("error Field = %q, want handle", d.Field)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateHandle(%q) returned error: %v", input, err)
			}
			if got != want {
				t.Errorf("validateHandle(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// A handle that would impersonate the platform must be refused even though
// it matches the pattern.
func TestValidateHandleRejectsReserved(t *testing.T) {
	for _, h := range []string{"admin", "Cuckoo", "system", "SUPPORT", "api"} {
		if _, err := validateHandle(h); err == nil {
			t.Errorf("validateHandle(%q) accepted a reserved handle", h)
		} else if domain.CodeOf(err) != "handle_reserved" {
			t.Errorf("validateHandle(%q) code = %q, want handle_reserved", h, domain.CodeOf(err))
		}
	}
}

func TestValidateDescription(t *testing.T) {
	if got, err := validateDescription("  Helps with banking.  "); err != nil || got != "Helps with banking." {
		t.Errorf("trim: got %q, %v", got, err)
	}
	if got, err := validateDescription(""); err != nil || got != "" {
		t.Errorf("empty must be allowed: got %q, %v", got, err)
	}
	if _, err := validateDescription(strings.Repeat("a", descriptionMaxLen)); err != nil {
		t.Errorf("at the limit rejected: %v", err)
	}
	if _, err := validateDescription(strings.Repeat("a", descriptionMaxLen+1)); err == nil {
		t.Error("over the limit accepted")
	}
	if _, err := validateDescription("line one\nline two"); err != nil {
		t.Errorf("descriptions may span lines: %v", err)
	}
}

func TestValidateWebhookURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/cuckoo":        true,
		"https://example.com":               true,
		"https://example.com:8443/hook?x=1": true,
		"http://localhost:9000/hook":        true,
		"http://127.0.0.1:9000/hook":        true,
		"http://[::1]:9000/hook":            true,

		"":                                false,
		"http://example.com/hook":         false, // plaintext to a real host
		"ftp://example.com/hook":          false,
		"example.com/hook":                false, // not absolute
		"/hook":                           false,
		"https://user:pass@example.com/h": false, // credentials in URL
		"https://example.com/hook#frag":   false,
		"not a url":                       false,
	}

	for input, ok := range cases {
		t.Run(input, func(t *testing.T) {
			_, err := validateWebhookURL(input)
			if ok && err != nil {
				t.Errorf("rejected valid URL: %v", err)
			}
			if !ok && err == nil {
				t.Error("accepted invalid URL")
			}
			if !ok && err != nil {
				if d, _ := domain.AsError(err); d.Field != "webhook_url" {
					t.Errorf("error Field = %q, want webhook_url", d.Field)
				}
			}
		})
	}
}
