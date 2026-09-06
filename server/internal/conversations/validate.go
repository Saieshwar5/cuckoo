package conversations

import (
	"fmt"
	"net/url"
	"regexp"
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

	// idempotencyKeyMaxLen leaves room for a UUID, a prefix and some
	// namespace, and no room for a key that is really a payload.
	idempotencyKeyMaxLen = 200

	// Button and quick-reply limits are the protocol's. A phone screen fits
	// three short buttons across; more rows or longer labels stop being a
	// choice and start being a form, which is a different feature.
	buttonRowsMax    = 3
	buttonsPerRowMax = 3
	quickRepliesMax  = 6
	labelMaxLen      = 40

	// buttonURLMaxLen is generous enough for a signed link with a token in
	// it and short enough that a url is not a payload.
	buttonURLMaxLen = 2048
	previewMaxLen   = 100
)

// buttonIDPattern: something a backend generates and matches on, not prose.
var buttonIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// buttonURLSchemes is what a button may open. The web, the way India pays,
// and the call every support line offers. Everything else is refused —
// javascript:, data:, file: and an app's private scheme are not things an
// unverified agent gets to put under a person's thumb.
var buttonURLSchemes = map[string]bool{"https": true, "http": true, "upi": true, "tel": true}

// validateButtons normalises and checks rows of buttons. Ids are unique
// within the message, since a tap names one of them.
func validateButtons(rows [][]Button) ([][]Button, error) {
	invalid := func(msg string) error { return domain.InvalidField("buttons", "invalid_buttons", msg) }
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) > buttonRowsMax {
		return nil, invalid(fmt.Sprintf("At most %d rows of buttons.", buttonRowsMax))
	}
	seen := map[string]bool{}
	out := make([][]Button, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 || len(row) > buttonsPerRowMax {
			return nil, invalid(fmt.Sprintf("Each row needs 1 to %d buttons.", buttonsPerRowMax))
		}
		clean := make([]Button, 0, len(row))
		for _, b := range row {
			btn, err := validateButton(b, seen)
			if err != nil {
				return nil, err
			}
			clean = append(clean, btn)
		}
		out = append(out, clean)
	}
	return out, nil
}

// validateButton checks one button and records its id, if it has one.
func validateButton(b Button, seen map[string]bool) (Button, error) {
	invalid := func(msg string) error { return domain.InvalidField("buttons", "invalid_buttons", msg) }

	label, err := validateLabel("buttons", "invalid_buttons", b.Label)
	if err != nil {
		return Button{}, err
	}
	style, err := validateButtonStyle(b.Style)
	if err != nil {
		return Button{}, err
	}

	if b.URL != "" {
		if b.ID != "" {
			return Button{}, invalid("A button has an id or a url, not both.")
		}
		url, err := validateButtonURL(b.URL)
		if err != nil {
			return Button{}, err
		}
		return Button{URL: url, Label: label, Style: style}, nil
	}

	if !buttonIDPattern.MatchString(b.ID) {
		return Button{}, invalid("Button ids are 1 to 64 characters: letters, digits, _ . : or -.")
	}
	if seen[b.ID] {
		return Button{}, invalid(fmt.Sprintf("Button id %q appears twice.", b.ID))
	}
	seen[b.ID] = true
	return Button{ID: b.ID, Label: label, Style: style}, nil
}

func validateButtonStyle(raw string) (string, error) {
	switch raw {
	case "":
		return ButtonDefault, nil
	case ButtonDefault, ButtonPrimary, ButtonDanger:
		return raw, nil
	}
	return "", domain.InvalidField("buttons", "invalid_buttons",
		`Button style must be "default", "primary" or "danger".`)
}

// validateButtonURL checks where a button leads. It is the one field in a
// message that acts on the person's device rather than being read, so it is
// checked for what it is rather than only for how long it is.
func validateButtonURL(raw string) (string, error) {
	invalid := func(msg string) error { return domain.InvalidField("buttons", "invalid_buttons", msg) }

	link := strings.TrimSpace(raw)
	if !utf8.ValidString(link) || strings.ContainsFunc(link, isURLBreaking) {
		return "", invalid("A button url cannot contain spaces, line breaks or control characters.")
	}
	if len(link) > buttonURLMaxLen {
		return "", invalid(fmt.Sprintf("A button url must be at most %d characters.", buttonURLMaxLen))
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", invalid("That button url is not a url.")
	}
	scheme := strings.ToLower(u.Scheme)
	if !buttonURLSchemes[scheme] {
		return "", invalid("A button url must start with https, http, upi or tel.")
	}
	// http and https address a host; tel carries a number and upi its
	// parameters. What is refused here is a scheme with nothing after it.
	if scheme == "https" || scheme == "http" {
		if u.Host == "" {
			return "", invalid("That button url has no address.")
		}
	} else if u.Host == "" && u.Opaque == "" {
		return "", invalid("That button url has no address.")
	}
	return link, nil
}

// isURLBreaking reports whether a rune must not appear in a url: spaces and
// control characters, which is how a link is dressed up as another one.
func isURLBreaking(r rune) bool {
	return r <= 0x20 || r == 0x7f || r == '\u2028' || r == '\u2029'
}

// validateQuickReplies normalises and checks suggested answers.
func validateQuickReplies(in []QuickReply) ([]QuickReply, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > quickRepliesMax {
		return nil, domain.InvalidField("quick_replies", "invalid_quick_replies",
			fmt.Sprintf("At most %d quick replies.", quickRepliesMax))
	}
	out := make([]QuickReply, 0, len(in))
	for _, q := range in {
		label, err := validateLabel("quick_replies", "invalid_quick_replies", q.Label)
		if err != nil {
			return nil, err
		}
		out = append(out, QuickReply{Label: label})
	}
	return out, nil
}

// validateLabel checks the short text on a button or chip.
func validateLabel(field, code, raw string) (string, error) {
	label := strings.TrimSpace(raw)
	if !utf8.ValidString(label) || strings.ContainsAny(label, "\n\r\t\x00") {
		return "", domain.InvalidField(field, code, "Labels cannot contain line breaks or control characters.")
	}
	if n := utf8.RuneCountInString(label); n < 1 || n > labelMaxLen {
		return "", domain.InvalidField(field, code, fmt.Sprintf("Labels must be 1 to %d characters.", labelMaxLen))
	}
	return label, nil
}

// validateCaption checks the words on a message. Text is required on a
// message that is only words, and optional on one carrying files: a photo
// with nothing written under it is a message, and an empty bubble is not.
func validateCaption(raw string, hasAttachments bool) (string, error) {
	if hasAttachments && strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return validateText(raw)
}

// previewOfBody is the short form of a message for a quote or a chat-list
// row: its words, or what it carried when there are none.
func previewOfBody(b Body) string {
	if strings.TrimSpace(b.Text) == "" {
		return previewOfAttachments(b.Attachments)
	}
	return previewOf(b.Text)
}

// previewOf is the short form of a message's text for a quote.
func previewOf(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= previewMaxLen {
		return text
	}
	return string([]rune(text)[:previewMaxLen]) + "…"
}

// validateIdempotencyKey returns nil for no key, so the column stays null and
// the unique index ignores the row.
func validateIdempotencyKey(raw string) (*string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil, nil
	}
	if !utf8.ValidString(key) || strings.ContainsRune(key, 0) ||
		utf8.RuneCountInString(key) > idempotencyKeyMaxLen {
		return nil, domain.InvalidField("idempotency_key", "invalid_idempotency_key",
			fmt.Sprintf("Idempotency key must be at most %d characters.", idempotencyKeyMaxLen))
	}
	return &key, nil
}

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
