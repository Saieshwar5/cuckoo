package signin

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

const (
	emailMaxLen      = 320
	deviceNameMaxLen = 80
	codeDigits       = 6
)

// normalizeEmail lowercases and checks an address. Case is folded so that
// Priya@Example.com and priya@example.com are one account, which is what
// every mail system in practice already does.
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	invalid := func() error {
		return domain.InvalidField("email", "invalid_email", "That does not look like an email address.")
	}
	if email == "" || len(email) > emailMaxLen || !utf8.ValidString(email) {
		return "", invalid()
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return "", invalid()
	}
	return email, nil
}

// validateDeviceName trims and bounds the optional device label.
func validateDeviceName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !utf8.ValidString(name) || strings.ContainsAny(name, "\n\r\t") ||
		utf8.RuneCountInString(name) > deviceNameMaxLen {
		return "", domain.InvalidField("device_name", "invalid_device_name",
			fmt.Sprintf("Device name must be at most %d characters on one line.", deviceNameMaxLen))
	}
	return name, nil
}

// newCode makes a six-digit code from the system's entropy source.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

// hashCode is the stored form of a code, salted with its row id so two
// people issued the same code do not share a hash.
func hashCode(id uuid.UUID, code string) []byte {
	sum := sha256.Sum256([]byte(id.String() + ":" + code))
	return sum[:]
}

// displayNameFor is a first name for a new account: the part before the @,
// which the person can change on their first screen.
func displayNameFor(email string) string {
	local, _, _ := strings.Cut(email, "@")
	local = strings.TrimSpace(local)
	if local == "" {
		return "New user"
	}
	if utf8.RuneCountInString(local) > 80 {
		local = string([]rune(local)[:80])
	}
	return local
}
