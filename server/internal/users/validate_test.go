package users

import (
	"strings"
	"testing"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

func TestValidateDisplayName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // empty means the input must be rejected
	}{
		{"simple", "Priya", "Priya"},
		{"trims surrounding space", "  Ravi  ", "Ravi"},
		{"keeps inner space", "Ravi Kumar", "Ravi Kumar"},
		{"telugu", "ప్రియ", "ప్రియ"},
		{"hindi", "रवि कुमार", "रवि कुमार"},
		{"emoji", "Ravi 👋", "Ravi 👋"},
		{"single character", "R", "R"},
		{"at the maximum", strings.Repeat("a", displayNameMaxLen), strings.Repeat("a", displayNameMaxLen)},

		{"empty", "", ""},
		{"only whitespace", "   ", ""},
		{"one over the maximum", strings.Repeat("a", displayNameMaxLen+1), ""},
		{"newline", "Ravi\nKumar", ""},
		{"carriage return", "Ravi\rKumar", ""},
		{"tab", "Ravi\tKumar", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateDisplayName(tc.input)

			if tc.want == "" {
				if err == nil {
					t.Fatalf("validateDisplayName(%q) accepted an invalid name", tc.input)
				}
				if domain.KindOf(err) != domain.KindInvalid {
					t.Errorf("error Kind = %q, want %q", domain.KindOf(err), domain.KindInvalid)
				}
				var d *domain.Error
				if d, _ = domain.AsError(err); d.Field != "display_name" {
					t.Errorf("error Field = %q, want display_name", d.Field)
				}
				return
			}

			if err != nil {
				t.Fatalf("validateDisplayName(%q) returned error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("validateDisplayName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Length is counted in characters, not bytes: a Telugu name of 80 characters
// weighs far more than 80 bytes, and rejecting it would make the limit mean
// something different in every language.
func TestDisplayNameLimitCountsCharactersNotBytes(t *testing.T) {
	name := strings.Repeat("ప", displayNameMaxLen)

	if len(name) <= displayNameMaxLen {
		t.Fatalf("test is not exercising multi-byte input: %d bytes", len(name))
	}

	if _, err := validateDisplayName(name); err != nil {
		t.Errorf("an %d-character Telugu name was rejected: %v", displayNameMaxLen, err)
	}
}

func TestValidateLocale(t *testing.T) {
	valid := []string{"en", "en-IN", "hi", "hi-IN", "te", "te-IN", "ta-IN", "bn"}
	for _, locale := range valid {
		t.Run("valid/"+locale, func(t *testing.T) {
			got, err := validateLocale(locale)
			if err != nil {
				t.Fatalf("validateLocale(%q) returned error: %v", locale, err)
			}
			if got != locale {
				t.Errorf("validateLocale(%q) = %q", locale, got)
			}
		})
	}

	invalid := []string{"", "e", "english", "EN", "en_IN", "en-in", "en-INDIA", "en IN", "12"}
	for _, locale := range invalid {
		t.Run("invalid/"+locale, func(t *testing.T) {
			if _, err := validateLocale(locale); err == nil {
				t.Fatalf("validateLocale(%q) accepted an invalid locale", locale)
			}
		})
	}
}

func TestValidateLocaleTrimsSpace(t *testing.T) {
	got, err := validateLocale("  en-IN  ")
	if err != nil {
		t.Fatalf("validateLocale returned error: %v", err)
	}
	if got != "en-IN" {
		t.Errorf("validateLocale = %q, want en-IN", got)
	}
}
