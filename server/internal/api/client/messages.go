package client

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

type senderResponse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// bodyResponse is the message content. It is an object, not a string, so
// attachments can join without changing the shape. Buttons and quick
// replies are what an agent offered; Action is what the person's tap
// chose; SelectedButtonID on the offering message is the button taken, so
// the app can dim the row.
type bodyResponse struct {
	Text             string               `json:"text,omitempty"`
	Attachments      []attachmentResponse `json:"attachments,omitempty"`
	Buttons          [][]buttonResponse   `json:"buttons,omitempty"`
	QuickReplies     []quickReplyResponse `json:"quick_replies,omitempty"`
	Action           *actionResponse      `json:"action,omitempty"`
	SelectedButtonID string               `json:"selected_button_id,omitempty"`
}

// attachmentResponse is a file the message carries. The bytes come from
// /media/{media_id}; the size and shape are here so the bubble is drawn
// correctly before they do.
type attachmentResponse struct {
	MediaID      string  `json:"media_id"`
	Kind         string  `json:"kind"`
	MimeType     string  `json:"mime_type"`
	ByteSize     int64   `json:"byte_size"`
	FileName     string  `json:"file_name"`
	Width        int32   `json:"width,omitempty"`
	Height       int32   `json:"height,omitempty"`
	HasThumbnail bool    `json:"has_thumbnail,omitempty"`
	DurationMS   int32   `json:"duration_ms,omitempty"`
	Waveform     []int32 `json:"waveform,omitempty"`
}

// buttonResponse is a button as the app draws it: a choice with an id, or a
// link with a url that opens somewhere outside Cuckoo.
type buttonResponse struct {
	ID    string `json:"id,omitempty"`
	URL   string `json:"url,omitempty"`
	Label string `json:"label"`
	Style string `json:"style"`
}

type quickReplyResponse struct {
	Label string `json:"label"`
}

type actionResponse struct {
	ButtonID        string `json:"button_id"`
	SourceMessageID string `json:"source_message_id"`
}

// replyToResponse is the quoted message, enough to draw it.
type replyToResponse struct {
	ID          string `json:"id"`
	SenderKind  string `json:"sender_kind"`
	TextPreview string `json:"text_preview"`
}

func newBodyResponse(b conversations.Body) bodyResponse {
	out := bodyResponse{Text: b.Text, SelectedButtonID: b.SelectedButtonID}
	for _, a := range b.Attachments {
		out.Attachments = append(out.Attachments, attachmentResponse{
			MediaID:      domain.FormatID(domain.PrefixMedia, a.MediaID),
			Kind:         a.Kind,
			MimeType:     a.MimeType,
			ByteSize:     a.ByteSize,
			FileName:     a.FileName,
			Width:        a.Width,
			Height:       a.Height,
			HasThumbnail: a.HasThumbnail,
			DurationMS:   a.DurationMS,
			Waveform:     a.Waveform,
		})
	}
	for _, row := range b.Buttons {
		wire := make([]buttonResponse, 0, len(row))
		for _, btn := range row {
			wire = append(wire, buttonResponse{ID: btn.ID, URL: btn.URL, Label: btn.Label, Style: btn.Style})
		}
		out.Buttons = append(out.Buttons, wire)
	}
	for _, q := range b.QuickReplies {
		out.QuickReplies = append(out.QuickReplies, quickReplyResponse{Label: q.Label})
	}
	if b.Action != nil {
		out.Action = &actionResponse{
			ButtonID:        b.Action.ButtonID,
			SourceMessageID: domain.FormatID(domain.PrefixMessage, b.Action.SourceMessageID),
		}
	}
	return out
}

// messageResponse is a message as the app shows it. DeliveryStatus is the
// tick mark: pending, delivered or failed for a message the caller sent to
// agents, and null for one with nobody to deliver to.
type messageResponse struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversation_id"`
	Sender         senderResponse   `json:"sender"`
	Body           bodyResponse     `json:"body"`
	ReplyTo        *replyToResponse `json:"reply_to"`
	Status         string           `json:"status"`
	Truncated      bool             `json:"truncated"`
	// Stopped: the person pressed stop while the agent was writing this.
	Stopped        bool      `json:"stopped"`
	DeliveryStatus *string   `json:"delivery_status"`
	CreatedAt      time.Time `json:"created_at"`
}

type messageEnvelope struct {
	Message messageResponse `json:"message"`
}

// messagePageEnvelope is one page of history. Paging back, NextBefore is the
// value to pass as ?before= for older messages; paging forward with ?after=,
// NextAfter continues. Null means the history ends there.
type messagePageEnvelope struct {
	Messages   []messageResponse `json:"messages"`
	NextBefore *string           `json:"next_before"`
	NextAfter  *string           `json:"next_after"`
	// Trimmed says the conversation began before the hub's window: with
	// NextBefore null, history ended because older messages expired, not
	// because there were none.
	Trimmed bool `json:"trimmed"`
}

func newMessageResponse(m conversations.Message) messageResponse {
	resp := messageResponse{
		ID:             domain.FormatID(domain.PrefixMessage, m.ID),
		ConversationID: domain.FormatID(domain.PrefixConv, m.ConversationID),
		Sender: senderResponse{
			Kind: string(m.Sender.Kind),
			ID:   formatParticipantID(m.Sender.Kind, m.Sender.ID),
		},
		Body:      newBodyResponse(m.Body),
		Status:    string(m.Status),
		Truncated: m.Truncated,
		Stopped:   m.Stopped,
		CreatedAt: m.CreatedAt,
	}
	if m.DeliveryStatus != "" {
		status := string(m.DeliveryStatus)
		resp.DeliveryStatus = &status
	}
	if m.ReplyTo != nil {
		resp.ReplyTo = &replyToResponse{
			ID:          domain.FormatID(domain.PrefixMessage, m.ReplyTo.ID),
			SenderKind:  string(m.ReplyTo.SenderKind),
			TextPreview: m.ReplyTo.TextPreview,
		}
	}
	return resp
}

// sendMessageRequest is a send. The idempotency key is optional on the wire
// and always sent by the app: a retry after a dropped response returns the
// message already created rather than a duplicate bubble. Action, instead
// of text, is a tap on a button an agent offered.
type sendMessageRequest struct {
	Text           string         `json:"text"`
	Attachments    []string       `json:"attachments"`
	IdempotencyKey string         `json:"idempotency_key"`
	ReplyTo        string         `json:"reply_to"`
	Action         *actionRequest `json:"action"`
}

type actionRequest struct {
	ButtonID        string `json:"button_id"`
	SourceMessageID string `json:"source_message_id"`
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	var req sendMessageRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	attachments, err := mediaIDsOf(req.Attachments)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := conversations.SendInput{
		Text: req.Text, Attachments: attachments, IdempotencyKey: req.IdempotencyKey,
	}
	if req.ReplyTo != "" {
		id, err := domain.ParseID(domain.PrefixMessage, req.ReplyTo)
		if err != nil {
			httpx.Error(w, r, domain.InvalidField("reply_to", "invalid_id", "reply_to must be a message id."))
			return
		}
		in.ReplyTo = &id
	}
	if req.Action != nil {
		source, err := domain.ParseID(domain.PrefixMessage, req.Action.SourceMessageID)
		if err != nil {
			httpx.Error(w, r, domain.InvalidField("action", "invalid_id", "action.source_message_id must be a message id."))
			return
		}
		in.Action = &conversations.Action{ButtonID: req.Action.ButtonID, SourceMessageID: source}
	}
	res, err := h.conversations.SendAsUser(r.Context(), userID, conversationID, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}
	httpx.JSON(w, r, status, messageEnvelope{Message: newMessageResponse(res.Message)})
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	before, err := httpx.QueryID(r, "before", domain.PrefixMessage)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	after, err := httpx.QueryID(r, "after", domain.PrefixMessage)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, err := httpx.QueryInt(r, "limit")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	page, err := h.conversations.ListMessages(r.Context(), userID, conversationID,
		conversations.ListMessagesInput{Before: before, After: after, Limit: limit})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	out := messagePageEnvelope{
		Messages: make([]messageResponse, 0, len(page.Messages)),
		Trimmed:  h.retention != nil && h.retention.Trimmed(page.ConversationStarted),
	}
	for _, m := range page.Messages {
		out.Messages = append(out.Messages, newMessageResponse(m))
	}
	if page.NextBefore != nil {
		cursor := domain.FormatID(domain.PrefixMessage, *page.NextBefore)
		out.NextBefore = &cursor
	}
	if page.NextAfter != nil {
		cursor := domain.FormatID(domain.PrefixMessage, *page.NextAfter)
		out.NextAfter = &cursor
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
