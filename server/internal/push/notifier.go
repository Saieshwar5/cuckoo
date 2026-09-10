package push

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// bodyLimit is how much of a message a lock screen gets. Android truncates
// anyway; this keeps the request small and the preview honest.
const bodyLimit = 180

// channelID names the Android channel the app declares. Without a matching
// one the notification arrives with default behaviour and no sound.
const channelID = "messages"

// Notifier decides whether a message is worth interrupting somebody for, and
// sends it if it is.
//
// Every rule here is one that a person would otherwise feel as a broken app:
// a buzz for the message they just sent, a buzz for a chat they are reading,
// a buzz from an agent they muted. The last one matters most — mute is what
// stands between an agent that is too talkative and an agent that is blocked
// for good, and a mute that still buzzes is not a mute.
type Notifier struct {
	store    *store.Store
	presence realtime.Presence
	sender   Sender
	log      *slog.Logger
}

func New(st *store.Store, presence realtime.Presence, sender Sender, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{store: st, presence: presence, sender: sender, log: log.With("component", "push")}
}

// MessageLanded notifies whoever should hear about a finished message.
//
// It never returns an error to its caller: a notification that could not be
// sent must not fail the message that was. The message is already delivered
// and already on its way down every open socket; this is the extra.
func (n *Notifier) MessageLanded(ctx context.Context, in conversations.Landed) {
	if err := n.send(ctx, in); err != nil {
		n.log.WarnContext(ctx, "could not notify", "conversation", in.ConversationID, "error", err)
	}
}

func (n *Notifier) send(ctx context.Context, in conversations.Landed) error {
	// Nobody is told about their own message.
	away := make([]uuid.UUID, 0, len(in.Recipients))
	for _, id := range in.Recipients {
		if id != in.SenderID {
			away = append(away, id)
		}
	}
	if len(away) == 0 {
		return nil
	}

	// Somebody with the app open can already see it. Interrupting them as
	// well is how people learn to turn notifications off.
	watching, err := n.presence.Watching(ctx, away)
	if err != nil {
		// Presence unknown: notify rather than stay silent. An unnecessary
		// buzz is a worse app; a missing one is a broken promise.
		n.log.WarnContext(ctx, "presence unavailable; notifying anyway", "error", err)
		watching = nil
	}
	absent := make([]uuid.UUID, 0, len(away))
	for _, id := range away {
		if !watching[id] {
			absent = append(absent, id)
		}
	}
	if len(absent) == 0 {
		return nil
	}

	agentID := in.AgentID
	targets, err := n.store.PushTargets(ctx, gen.PushTargetsParams{
		AgentID: &agentID, UserIds: absent,
	})
	if err != nil {
		return fmt.Errorf("find devices: %w", err)
	}
	if len(targets) == 0 {
		return nil
	}

	body := preview(in.Text, in.HasAttachments)
	if body == "" {
		return nil
	}
	// The title is who said it. Looked up here rather than carried along the
	// message path, which has no need of a display name.
	title := "Cuckoo"
	if agent, err := n.store.GetAgent(ctx, in.AgentID); err == nil && agent.DisplayName != "" {
		title = agent.DisplayName
	}
	conversationID := domain.FormatID(domain.PrefixConv, in.ConversationID)

	messages := make([]Message, 0, len(targets))
	for _, t := range targets {
		if t.PushToken == nil {
			continue
		}
		messages = append(messages, Message{
			To:    *t.PushToken,
			Title: title,
			Body:  body,
			// What the app opens on a tap.
			Data:      map[string]string{"conversation_id": conversationID},
			Sound:     "default",
			ChannelID: channelID,
			// Ten messages in one chat are one line, not ten.
			CollapseKey: conversationID,
		})
	}
	if len(messages) == 0 {
		return nil
	}

	results, err := n.sender.Send(ctx, messages)
	if err != nil {
		return err
	}

	// A token Expo says is dead stays dead. Kept, it is a message sent into
	// nothing for as long as the account exists.
	for _, r := range results {
		if r.Gone {
			if err := n.store.ClearSessionPush(ctx, &r.Token); err != nil {
				n.log.WarnContext(ctx, "could not forget a dead device", "error", err)
			}
			continue
		}
		if r.Error != "" {
			n.log.WarnContext(ctx, "a notification was refused", "reason", r.Error)
		}
	}
	return nil
}

// preview is what appears on a lock screen.
//
// A message with only files still deserves a line: "sent a photo" is the
// difference between knowing something arrived and not.
func preview(text string, hasAttachments bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		if hasAttachments {
			return "sent a file"
		}
		return ""
	}
	// Cut on a rune boundary; a body ending in half a Telugu character is
	// worse than one ending early.
	if utf8.RuneCountInString(trimmed) <= bodyLimit {
		return trimmed
	}
	runes := []rune(trimmed)
	return strings.TrimSpace(string(runes[:bodyLimit])) + "…"
}
