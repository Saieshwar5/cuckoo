package conversations

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

const (
	// textMaxLen is the protocol's limit, in characters: long enough for any
	// reply an agent should be making in a chat bubble, short enough that
	// one message cannot be used as a file transfer.
	textMaxLen = 8000

	defaultPageSize int32 = 50
	maxPageSize     int32 = 100
)

// validateText normalises and checks message text.
func validateText(raw string) (string, error) {
	text := strings.TrimSpace(raw)

	if !utf8.ValidString(text) {
		return "", domain.InvalidField("text", "invalid_text",
			"Message contains characters we cannot read.")
	}
	// Postgres cannot store a NUL inside jsonb; refusing it here turns a
	// database error into a validation error the sender can act on.
	if strings.ContainsRune(text, 0) {
		return "", domain.InvalidField("text", "invalid_text",
			"Message contains characters we cannot store.")
	}
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return "", domain.InvalidField("text", "invalid_text",
			"Message text cannot be empty.")
	}
	if n > textMaxLen {
		return "", domain.InvalidField("text", "invalid_text",
			fmt.Sprintf("Message text must be at most %d characters.", textMaxLen))
	}
	return text, nil
}

// pageSize applies the default and refuses anything out of range. Rejecting
// rather than silently clamping means a client asking for 500 finds out now,
// not when its pagination loop misbehaves.
func pageSize(limit int) (int32, error) {
	if limit == 0 {
		return defaultPageSize, nil
	}
	if limit < 1 || limit > int(maxPageSize) {
		return 0, domain.InvalidField("limit", "invalid_limit",
			fmt.Sprintf("Limit must be between 1 and %d.", maxPageSize))
	}
	return int32(limit), nil
}
