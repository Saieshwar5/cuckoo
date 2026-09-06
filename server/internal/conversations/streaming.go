package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// streamIdleFor is how long a stream may go without a delta before the hub
// finishes it for the agent. Long enough for a model that pauses to call a
// tool; short enough that a bubble never spins for a minute.
const streamIdleFor = 30 * time.Second

// streamStuckFor is the fallback: a row still streaming this long after it
// began has lost its buffer bookkeeping and is finished from what remains.
const streamStuckFor = 5 * time.Minute

// StartStream begins an empty message from the agent in a conversation it
// is in. The message exists at once, with status streaming, so the person's
// device can draw the bubble; text arrives through AppendStream and the
// message is final after FinishStream.
func (s *Service) StartStream(ctx context.Context, agentID, conversationID uuid.UUID, in SendInput) (SendResult, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return SendResult{}, err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return SendResult{}, err
	}
	return s.create(ctx, Sender{Kind: ParticipantAgent, ID: agentID}, conversationID, in, agentSendPolicy, true)
}

// openStream creates the buffer for a message just created as streaming.
func (s *Service) openStream(ctx context.Context, msg Message) error {
	userIDs, err := s.store.ListUserParticipants(ctx, msg.ConversationID)
	if err != nil {
		return domain.Internal(fmt.Errorf("list recipients of stream %s: %w", msg.ID, err))
	}
	ref := streamRef{AgentID: msg.Sender.ID, MessageID: msg.ID}
	if err := s.streams.open(ctx, ref, msg.ConversationID, userIDs); err != nil {
		// The row says streaming and nothing will append to it; the stuck
		// sweep finishes it. Better a truncated message than a lost one.
		return domain.Internal(err)
	}
	return nil
}

// AppendStream adds text to a stream the agent started. No database is
// touched: the buffer knows the conversation and its people.
func (s *Service) AppendStream(ctx context.Context, agentID, messageID uuid.UUID, text string) error {
	if s.streams == nil {
		return domain.Internal(errors.New("streaming is not configured"))
	}
	if text == "" {
		return domain.InvalidField("text", "invalid_text", "A delta cannot be empty.")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return domain.InvalidField("text", "invalid_text", "Delta contains characters we cannot store.")
	}

	wait, err := s.limiter.Allow(ctx, "delta:agent:"+agentID.String(), agentDeltaPolicy)
	if err != nil {
		return domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return domain.RateLimited("rate_limited", "You are streaming too quickly.", wait)
	}

	ref := streamRef{AgentID: agentID, MessageID: messageID}
	conversationID, userIDs, err := s.streams.append(ctx, ref, text)
	switch {
	case errors.Is(err, errNotStreaming):
		return domain.Conflict("not_streaming",
			"That message is not streaming. It may have finished, been cut off, or belong to another agent.")
	case errors.Is(err, errStreamTooLong):
		return domain.Invalid("stream_too_long",
			fmt.Sprintf("The message has reached %d characters; finish it.", textMaxLen))
	case err != nil:
		return domain.Internal(err)
	}

	s.notifyTo(ctx, EventMessageDelta, userIDs, MessageDeltaEvent{
		ConversationID: conversationID, MessageID: messageID, Text: text,
	})
	return nil
}

// FinishStream ends a stream the agent started: the buffered text becomes
// the message's body in one update, with any buttons or quick replies, the
// agents in the conversation are told about the whole message, and the
// person's device is told it is final. Finishing twice returns the finished
// message.
func (s *Service) FinishStream(ctx context.Context, agentID, messageID uuid.UUID, in FinishInput) (Message, error) {
	buttons, err := validateButtons(in.Buttons)
	if err != nil {
		return Message{}, err
	}
	quick, err := validateQuickReplies(in.QuickReplies)
	if err != nil {
		return Message{}, err
	}
	return s.finishStream(ctx, agentID, messageID, FinishInput{Buttons: buttons, QuickReplies: quick}, false)
}

var errAlreadyFinished = errors.New("stream: already finished")

func (s *Service) finishStream(ctx context.Context, agentID, messageID uuid.UUID, in FinishInput, cutOff bool) (Message, error) {
	if s.streams == nil {
		return Message{}, domain.Internal(errors.New("streaming is not configured"))
	}
	ref := streamRef{AgentID: agentID, MessageID: messageID}
	text, _, err := s.streams.text(ctx, ref)
	if err != nil {
		return Message{}, domain.Internal(err)
	}
	text, clamped := clampText(text)
	body, err := json.Marshal(Body{Text: text, Buttons: in.Buttons, QuickReplies: in.QuickReplies})
	if err != nil {
		return Message{}, domain.Internal(fmt.Errorf("encode message body: %w", err))
	}

	var (
		msg     Message
		pending []uuid.UUID
	)
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		row, err := tx.FinishMessage(ctx, gen.FinishMessageParams{
			ID: messageID, SenderAgentID: agentID, Body: body, Truncated: cutOff || clamped,
		})
		if err != nil {
			if store.IsNoRows(err) {
				return errAlreadyFinished
			}
			return domain.Internal(fmt.Errorf("finish message %s: %w", messageID, err))
		}
		if msg, err = messageFromRow(row); err != nil {
			return err
		}
		pending, err = fanOut(ctx, tx, msg)
		return err
	})
	if errors.Is(err, errAlreadyFinished) {
		return s.ownFinishedMessage(ctx, agentID, messageID)
	}
	if err != nil {
		if _, classified := domain.AsError(err); classified {
			return Message{}, err
		}
		return Message{}, domain.Internal(fmt.Errorf("finish stream: %w", err))
	}

	if err := s.streams.remove(ctx, ref); err != nil {
		slog.WarnContext(ctx, "stream: buffer not removed after finish", "message", messageID, "error", err)
	}
	final := []Message{msg}
	if err := s.decorate(ctx, final, true); err != nil {
		return Message{}, err
	}
	s.notify(ctx, EventMessageCompleted, msg.ConversationID,
		MessageCreatedEvent{ConversationID: msg.ConversationID, Message: final[0]})
	s.nudge(ctx, pending)
	return final[0], nil
}

// ownFinishedMessage answers a repeated finish: the message, if it is this
// agent's and complete; otherwise it is not this agent's to finish.
func (s *Service) ownFinishedMessage(ctx context.Context, agentID, messageID uuid.UUID) (Message, error) {
	row, err := s.store.GetMessage(ctx, messageID)
	if err != nil {
		if store.IsNoRows(err) {
			return Message{}, errNotOwnStream()
		}
		return Message{}, domain.Internal(fmt.Errorf("get message %s: %w", messageID, err))
	}
	msg, err := messageFromRow(row)
	if err != nil {
		return Message{}, err
	}
	if msg.Sender.Kind != ParticipantAgent || msg.Sender.ID != agentID || msg.Status != MessageComplete {
		return Message{}, errNotOwnStream()
	}
	return msg, nil
}

func errNotOwnStream() error {
	return domain.NotFound("stream_not_found", "No stream of yours has that message id.")
}

// SweepStreams finishes, as truncated, every stream idle for idleFor and
// every row stuck streaming past its buffer's life. It returns how many it
// finished.
func (s *Service) SweepStreams(ctx context.Context, idleFor time.Duration) (int, error) {
	if s.streams == nil {
		return 0, nil
	}
	refs, err := s.streams.idle(ctx, idleFor)
	if err != nil {
		return 0, domain.Internal(err)
	}
	stale, err := s.store.ListStaleStreamingMessages(ctx, time.Now().Add(-streamStuckFor))
	if err != nil {
		return 0, domain.Internal(fmt.Errorf("list stale streams: %w", err))
	}
	for _, r := range stale {
		if r.SenderAgentID != nil {
			refs = append(refs, streamRef{AgentID: *r.SenderAgentID, MessageID: r.ID})
		}
	}

	finished := 0
	for _, ref := range refs {
		if _, err := s.finishStream(ctx, ref.AgentID, ref.MessageID, FinishInput{}, true); err != nil {
			// Already finished by the agent between listing and now, or by
			// another instance's sweep: forget the bookkeeping and move on.
			_ = s.streams.remove(ctx, ref)
			continue
		}
		finished++
	}
	return finished, nil
}

// RunStreamSweeper sweeps every interval until ctx ends.
func (s *Service) RunStreamSweeper(ctx context.Context, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.SweepStreams(ctx, streamIdleFor)
			if err != nil && ctx.Err() == nil {
				log.Error("stream sweep failed", "error", err)
			}
			if n > 0 {
				log.Warn("streams cut off for going quiet", "count", n)
			}
		}
	}
}

// attachStreamText fills in the text so far of messages still streaming,
// for a reader who arrives mid-stream.
func (s *Service) attachStreamText(ctx context.Context, msgs []Message) error {
	if s.streams == nil {
		return nil
	}
	var refs []streamRef
	for _, m := range msgs {
		if m.Status == MessageStreaming && m.Sender.Kind == ParticipantAgent {
			refs = append(refs, streamRef{AgentID: m.Sender.ID, MessageID: m.ID})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	texts, err := s.streams.texts(ctx, refs)
	if err != nil {
		return domain.Internal(err)
	}
	for i := range msgs {
		if text, ok := texts[msgs[i].ID]; ok {
			msgs[i].Body.Text = text
		}
	}
	return nil
}

// notifyTo publishes to a known set of people, for events whose recipients
// were resolved when the stream was opened.
func (s *Service) notifyTo(ctx context.Context, eventType string, userIDs []uuid.UUID, payload any) {
	if len(userIDs) == 0 {
		return
	}
	ev, err := realtime.NewEvent(eventType, userIDs, payload)
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not encode event", "event", eventType, "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "realtime: could not publish", "event", eventType, "error", err)
	}
}

// clampText trims a stream's text to the message limit, reporting whether
// anything was lost.
func clampText(text string) (string, bool) {
	if utf8.RuneCountInString(text) <= textMaxLen {
		return text, false
	}
	runes := []rune(text)
	return string(runes[:textMaxLen]), true
}
