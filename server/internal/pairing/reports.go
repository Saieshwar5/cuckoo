package pairing

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Reasons a person may give. Short, because the note is where the story
// goes, and fixed, because a queue read by hand needs sorting into piles.
var reportReasons = map[string]bool{"spam": true, "impersonation": true, "abuse": true, "other": true}

const (
	reportNoteMaxLen = 500
	// reportOnceIn is how long one report of one agent stands for: a second
	// within it adds nothing to the pile, so it is refused rather than
	// filed twice.
	reportOnceIn = 24 * time.Hour
)

// ReportInput is what the person says about an agent.
type ReportInput struct {
	Reason string
	Note   string
	// MessageID, when set, names the message that prompted this, which must
	// be in the person's chat with the agent.
	MessageID *uuid.UUID
}

// Report files a complaint about an agent the caller has met — one in
// their list, or one that was. The agent is not told. Nothing else happens
// automatically: someone reads the pile.
func (s *Service) Report(ctx context.Context, callerID, agentID uuid.UUID, in ReportInput) error {
	contact, err := s.store.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return domain.NotFound("not_a_contact", "That agent is not in your list.")
		}
		return domain.Internal(fmt.Errorf("get contact: %w", err))
	}
	if !reportReasons[in.Reason] {
		return domain.InvalidField("reason", "invalid_reason", `Reason must be "spam", "impersonation", "abuse" or "other".`)
	}
	note := strings.TrimSpace(in.Note)
	if !utf8.ValidString(note) || strings.ContainsRune(note, 0) || utf8.RuneCountInString(note) > reportNoteMaxLen {
		return domain.InvalidField("note", "invalid_note", fmt.Sprintf("The note must be at most %d characters.", reportNoteMaxLen))
	}
	if in.MessageID != nil {
		msg, err := s.store.GetMessage(ctx, *in.MessageID)
		if err != nil || msg.ConversationID != contact.DmConversationID {
			return domain.InvalidField("message_id", "invalid_message_id", "message_id must be a message in your chat with this agent.")
		}
	}
	n, err := s.store.CountRecentReports(ctx, gen.CountRecentReportsParams{
		ReporterUserID: callerID, AgentID: agentID, Since: time.Now().Add(-reportOnceIn),
	})
	if err != nil {
		return domain.Internal(fmt.Errorf("count reports: %w", err))
	}
	if n > 0 {
		return domain.Conflict("already_reported", "You reported this agent recently. It is being looked at.")
	}
	if _, err := s.store.CreateReport(ctx, gen.CreateReportParams{
		ID: domain.NewID(), ReporterUserID: callerID, AgentID: agentID,
		MessageID: in.MessageID, Reason: in.Reason, Note: note,
	}); err != nil {
		return domain.Internal(fmt.Errorf("create report: %w", err))
	}
	return nil
}
