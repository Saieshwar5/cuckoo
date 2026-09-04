package users

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/cuckoo-chat/cuckoo/server/internal/domain"
)

// DefaultLocale is used when an account is created without one. India first,
// so en-IN rather than en-US: it selects Indian date, number and currency
// formatting even before the interface is translated.
const DefaultLocale = "en-IN"

const (
	displayNameMinLen = 1
	displayNameMaxLen = 80
)

// localePattern accepts a language tag with an optional region: en, en-IN, hi,
// te-IN. Deliberately narrow — the value reaches formatting code, and a wide
// grammar here would mean validating it again everywhere it is read.
var localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

// validateDisplayName normalises and checks a name, returning the value to
// store. Length is counted in runes: "ప్రియ" is five characters to the person
// who typed it, whatever it weighs in bytes.
func validateDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)

	if !utf8.ValidString(name) {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			"Name contains characters we cannot read.")
	}

	if n := utf8.RuneCountInString(name); n < displayNameMinLen || n > displayNameMaxLen {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			fmt.Sprintf("Name must be between %d and %d characters.",
				displayNameMinLen, displayNameMaxLen))
	}

	if strings.ContainsAny(name, "\n\r\t") {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			"Name cannot contain line breaks.")
	}

	return name, nil
}

// validateLocale checks a BCP 47 style language tag.
func validateLocale(raw string) (string, error) {
	locale := strings.TrimSpace(raw)
	if !localePattern.MatchString(locale) {
		return "", domain.InvalidField("locale", "invalid_locale",
			"Locale must look like \"en\" or \"en-IN\".")
	}
	return locale, nil
}
