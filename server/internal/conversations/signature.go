package conversations

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// signedContent is what the hub's signature covers: the message's identity
// and everything about it that never changes. Which button was later tapped
// is a fact recorded on the message afterwards, and is left out; so is the
// time, because the id already carries it.
//
// Field order is the wire order, and the wire order is the contract:
// changing it would make every signature so far unverifiable.
type signedContent struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversation_id"`
	SenderKind     string       `json:"sender_kind"`
	SenderID       string       `json:"sender_id"`
	Text           string       `json:"text"`
	Attachments    []string     `json:"attachments"`
	Buttons        [][]Button   `json:"buttons"`
	QuickReplies   []QuickReply `json:"quick_replies"`
	Action         *Action      `json:"action"`
	ReplyTo        string       `json:"reply_to"`
	Truncated      bool         `json:"truncated"`
}

// signedPayload is the bytes a message's signature is computed over.
func signedPayload(m Message) ([]byte, error) {
	c := signedContent{
		ID:             m.ID.String(),
		ConversationID: m.ConversationID.String(),
		SenderKind:     string(m.Sender.Kind),
		SenderID:       m.Sender.ID.String(),
		Text:           m.Body.Text,
		Attachments:    make([]string, 0, len(m.Body.Attachments)),
		Buttons:        m.Body.Buttons,
		QuickReplies:   m.Body.QuickReplies,
		Action:         m.Body.Action,
		Truncated:      m.Truncated,
	}
	for _, a := range m.Body.Attachments {
		c.Attachments = append(c.Attachments, a.MediaID.String())
	}
	if m.ReplyTo != nil && m.ReplyTo.ID != uuid.Nil {
		c.ReplyTo = m.ReplyTo.ID.String()
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("encode message %s for signing: %w", m.ID, err))
	}
	return raw, nil
}

// sign returns the hub's signature over a message, or nothing when the
// service has no signer.
func (s *Service) sign(m Message) ([]byte, error) {
	if s.signer == nil {
		return nil, nil
	}
	payload, err := signedPayload(m)
	if err != nil {
		return nil, err
	}
	return s.signer.Sign(payload), nil
}

// Verify reports whether a message carries this hub's own signature over
// its content. A message with no signature does not verify.
func (s *Service) Verify(m Message) bool {
	if s.signer == nil {
		return false
	}
	payload, err := signedPayload(m)
	if err != nil {
		return false
	}
	return s.signer.Verify(payload, m.Signature)
}
