package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// decorate fills in what a stored row does not carry: the text so far of a
// stream, the quote of a reply, and, for the sender's own view, delivery
// status. Every path that hands messages out goes through it.
func (s *Service) decorate(ctx context.Context, msgs []Message, forSender bool) error {
	if forSender {
		if err := s.attachDeliveryStatus(ctx, msgs); err != nil {
			return err
		}
	}
	if err := s.attachStreamText(ctx, msgs); err != nil {
		return err
	}
	if err := s.attachScheduleTitles(ctx, msgs); err != nil {
		return err
	}
	return s.attachReplyPreviews(ctx, msgs)
}

// attachReplyPreviews reads the messages a batch quotes, in one query, and
// fills in who wrote them and a short form of what they said.
func (s *Service) attachReplyPreviews(ctx context.Context, msgs []Message) error {
	var ids []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, m := range msgs {
		if m.ReplyTo != nil && !seen[m.ReplyTo.ID] {
			seen[m.ReplyTo.ID] = true
			ids = append(ids, m.ReplyTo.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := s.store.ListMessagesByIDs(ctx, ids)
	if err != nil {
		return domain.Internal(fmt.Errorf("list quoted messages: %w", err))
	}
	originals := make(map[uuid.UUID]Message, len(rows))
	for _, r := range rows {
		orig, err := messageFromRow(r)
		if err != nil {
			return err
		}
		originals[orig.ID] = orig
	}
	for i := range msgs {
		if msgs[i].ReplyTo == nil {
			continue
		}
		if orig, ok := originals[msgs[i].ReplyTo.ID]; ok {
			msgs[i].ReplyTo.SenderKind = orig.Sender.Kind
			msgs[i].ReplyTo.TextPreview = previewOfBody(orig.Body)
		}
	}
	return nil
}
