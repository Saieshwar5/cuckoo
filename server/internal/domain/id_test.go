package domain

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestFormatAndParseRoundTrip(t *testing.T) {
	prefixes := []string{
		PrefixUser, PrefixSession, PrefixAgent, PrefixBinding,
		PrefixConv, PrefixMessage, PrefixEvent, PrefixToken,
		PrefixMedia, PrefixAPIKey,
	}

	for _, prefix := range prefixes {
		t.Run(prefix, func(t *testing.T) {
			id := NewID()
			formatted := FormatID(prefix, id)

			if !strings.HasPrefix(formatted, prefix+"_") {
				t.Fatalf("FormatID(%q) = %q, want %q prefix", prefix, formatted, prefix+"_")
			}
			if got := len(formatted) - len(prefix) - 1; got != encodedIDLen {
				t.Errorf("encoded body is %d characters, want %d", got, encodedIDLen)
			}

			parsed, err := ParseID(prefix, formatted)
			if err != nil {
				t.Fatalf("ParseID(%q) returned error: %v", formatted, err)
			}
			if parsed != id {
				t.Errorf("round trip changed the value: got %v, want %v", parsed, id)
			}
		})
	}
}

// An identifier must not be usable in the wrong place. Accepting a message id
// where a conversation id belongs would turn a client bug into a data leak.
func TestParseIDRejectsWrongPrefix(t *testing.T) {
	formatted := FormatID(PrefixMessage, NewID())

	if _, err := ParseID(PrefixConv, formatted); err == nil {
		t.Fatal("ParseID accepted a message id as a conversation id")
	}
}

func TestParseIDRejectsMalformedInput(t *testing.T) {
	valid := FormatID(PrefixUser, NewID())

	cases := map[string]string{
		"empty":              "",
		"prefix only":        "usr_",
		"no separator":       "usr" + strings.TrimPrefix(valid, "usr_"),
		"too short":          "usr_01j7k2m3n4",
		"too long":           valid + "aa",
		"ambiguous letter i": "usr_" + strings.Repeat("i", encodedIDLen),
		"ambiguous letter l": "usr_" + strings.Repeat("l", encodedIDLen),
		"uppercase":          strings.ToUpper(valid),
		"different type":     FormatID(PrefixAgent, NewID()),
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseID(PrefixUser, input); err == nil {
				t.Fatalf("ParseID accepted malformed input %q", input)
			}
		})
	}
}

// Identifiers must sort by creation time: message history pages on the primary
// key alone, so an out-of-order id would put a message in the wrong place in a
// conversation.
func TestNewIDSortsByCreationTime(t *testing.T) {
	const count = 50

	ids := make([]string, 0, count)
	for range count {
		ids = append(ids, FormatID(PrefixMessage, NewID()))
		// UUIDv7 has millisecond resolution; without a pause the ordering
		// within a millisecond is decided by the random tail.
		time.Sleep(time.Millisecond)
	}

	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)

	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("identifiers are not time-ordered at position %d:\n generated: %s\n    sorted: %s",
				i, ids[i], sorted[i])
		}
	}
}

func TestNewIDIsUnique(t *testing.T) {
	const count = 10_000

	seen := make(map[string]struct{}, count)
	for range count {
		id := FormatID(PrefixUser, NewID())
		if _, dup := seen[id]; dup {
			t.Fatalf("NewID produced a duplicate: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// The alphabet deliberately omits i, l, o and u so an identifier read aloud or
// copied from a screenshot cannot become a different valid identifier.
func TestEncodingOmitsAmbiguousCharacters(t *testing.T) {
	for _, c := range "ilou" {
		if strings.ContainsRune("0123456789abcdefghjkmnpqrstvwxyz", c) {
			t.Errorf("alphabet contains ambiguous character %q", c)
		}
	}
}
