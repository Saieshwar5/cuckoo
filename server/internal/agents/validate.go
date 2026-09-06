package agents

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

const (
	displayNameMaxLen = 80
	descriptionMaxLen = 500
)

// handlePattern: lowercase, starts alphanumeric, 3–32 characters. The same
// rule is a CHECK constraint on the table, so the database refuses what this
// misses.
var handlePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

// reservedHandles can never be claimed. They would either impersonate the
// platform or collide with routes and identifiers we may need later.
var reservedHandles = map[string]bool{
	"admin": true, "administrator": true, "cuckoo": true, "system": true,
	"support": true, "help": true, "root": true, "api": true, "hub": true,
	"official": true, "staff": true, "team": true, "null": true, "undefined": true,
}

// validateHandle normalises and checks a handle. Case is folded rather than
// rejected: a handle is an identifier people type, and "MyHelper" and
// "myhelper" naming different agents would be a trap.
func validateHandle(raw string) (string, error) {
	handle := strings.ToLower(strings.TrimSpace(raw))

	if !handlePattern.MatchString(handle) {
		return "", domain.InvalidField("handle", "invalid_handle",
			"Handle must be 3–32 characters: lowercase letters, digits, _ or -, starting with a letter or digit.")
	}
	if reservedHandles[handle] {
		return "", domain.InvalidField("handle", "handle_reserved",
			fmt.Sprintf("The handle %q is reserved.", handle))
	}
	return handle, nil
}

// validateDisplayName mirrors the rule for people's names. Counted in
// characters, not bytes, so the limit means the same thing in every script.
func validateDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)

	if !utf8.ValidString(name) {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			"Name contains characters we cannot read.")
	}
	if n := utf8.RuneCountInString(name); n < 1 || n > displayNameMaxLen {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			fmt.Sprintf("Name must be between 1 and %d characters.", displayNameMaxLen))
	}
	if strings.ContainsAny(name, "\n\r\t") {
		return "", domain.InvalidField("display_name", "invalid_display_name",
			"Name cannot contain line breaks.")
	}
	return name, nil
}

// validateDescription allows empty and trims; it may span lines.
func validateDescription(raw string) (string, error) {
	desc := strings.TrimSpace(raw)

	if !utf8.ValidString(desc) {
		return "", domain.InvalidField("description", "invalid_description",
			"Description contains characters we cannot read.")
	}
	if utf8.RuneCountInString(desc) > descriptionMaxLen {
		return "", domain.InvalidField("description", "invalid_description",
			fmt.Sprintf("Description must be at most %d characters.", descriptionMaxLen))
	}
	return desc, nil
}

// validateWebhookURL requires an absolute HTTPS URL. Plain HTTP is allowed
// only to a loopback address, so a developer can point an agent at a script on
// their own machine without a certificate — while a URL to anything else
// travels encrypted, because the hub will be posting users' messages to it.
func validateWebhookURL(raw string) (string, error) {
	invalid := func(msg string) error {
		return domain.InvalidField("webhook_url", "invalid_webhook_url", msg)
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", invalid("A webhook binding needs a URL.")
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !u.IsAbs() {
		return "", invalid("Webhook URL must be absolute, e.g. https://example.com/cuckoo.")
	}
	if u.User != nil {
		return "", invalid("Webhook URL must not contain credentials.")
	}
	if u.Fragment != "" {
		return "", invalid("Webhook URL must not contain a fragment.")
	}

	switch u.Scheme {
	case "https":
		return u.String(), nil
	case "http":
		if isLoopback(u.Hostname()) {
			return u.String(), nil
		}
		return "", invalid("Webhook URL must use https (http is allowed only for localhost).")
	default:
		return "", invalid("Webhook URL must use https.")
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

const (
	// startersMax is how many suggestions an empty chat shows. Four chips
	// fit a phone's width in two rows; more is a menu, which is a different
	// feature.
	startersMax   = 4
	starterMaxLen = 40
)

// validateStarters trims each suggestion and refuses too many, empty ones,
// long ones, line breaks, and repeats. Nil and empty both mean none.
func validateStarters(raw []string) ([]string, error) {
	invalid := func(msg string) error { return domain.InvalidField("starters", "invalid_starters", msg) }
	if len(raw) > startersMax {
		return nil, invalid(fmt.Sprintf("At most %d starters.", startersMax))
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		label := strings.TrimSpace(r)
		if !utf8.ValidString(label) || strings.ContainsAny(label, "\n\r\t\x00") {
			return nil, invalid("Starters cannot contain line breaks or control characters.")
		}
		if n := utf8.RuneCountInString(label); n < 1 || n > starterMaxLen {
			return nil, invalid(fmt.Sprintf("Each starter must be 1 to %d characters.", starterMaxLen))
		}
		if seen[label] {
			return nil, invalid(fmt.Sprintf("Starter %q appears twice.", label))
		}
		seen[label] = true
		out = append(out, label)
	}
	return out, nil
}

// startersJSON encodes a validated list for the jsonb column.
func startersJSON(list []string) []byte {
	if list == nil {
		list = []string{}
	}
	raw, _ := json.Marshal(list)
	return raw
}
