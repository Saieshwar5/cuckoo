package domain

import (
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Identifier prefixes. Every ID the API exposes is prefixed with the kind of
// thing it names, so a value pasted into a bug report or a log line is
// self-describing and a mistyped ID is rejected before it reaches the database.
const (
	PrefixUser     = "usr"
	PrefixSession  = "ses"
	PrefixAgent    = "agt"
	PrefixBinding  = "bnd"
	PrefixConv     = "cnv"
	PrefixMessage  = "msg"
	PrefixEvent    = "evt"
	PrefixToken    = "tok"
	PrefixMedia    = "med"
	PrefixAPIKey   = "key"
	PrefixSchedule = "sch"
)

// idEncoding is Crockford base32 in lowercase: no padding, and no characters
// that are easily confused by eye (i, l, o, u are absent). Sixteen raw bytes
// encode to exactly 26 characters.
var idEncoding = base32.
	NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").
	WithPadding(base32.NoPadding)

const encodedIDLen = 26

// NewID returns a fresh UUIDv7. Version 7 embeds a millisecond timestamp, so
// identifiers sort by creation time — which keeps B-tree inserts sequential and
// lets message history paginate on the primary key alone.
func NewID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 fails only if the system entropy source is broken, in which
		// case nothing else in the process is trustworthy either.
		panic(fmt.Sprintf("domain: cannot generate id: %v", err))
	}
	return id
}

// FormatID renders an identifier for the API, e.g. "usr_01j7k2m3n4p5q6r7s8t9v0w1x2".
func FormatID(prefix string, id uuid.UUID) string {
	return prefix + "_" + idEncoding.EncodeToString(id[:])
}

// ParseID reads an identifier produced by FormatID, requiring the expected
// prefix so a conversation ID can never be passed where a message ID belongs.
func ParseID(prefix, s string) (uuid.UUID, error) {
	want := prefix + "_"
	if !strings.HasPrefix(s, want) {
		return uuid.Nil, InvalidField("id", "invalid_id",
			fmt.Sprintf("Expected an identifier starting with %q.", want))
	}

	body := s[len(want):]
	if len(body) != encodedIDLen {
		return uuid.Nil, InvalidField("id", "invalid_id", "Identifier is the wrong length.")
	}

	raw, err := idEncoding.DecodeString(body)
	if err != nil || len(raw) != 16 {
		return uuid.Nil, InvalidField("id", "invalid_id", "Identifier is not valid.")
	}

	var id uuid.UUID
	copy(id[:], raw)
	return id, nil
}
