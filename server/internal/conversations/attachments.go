package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// attachmentsMax is how many files one message may carry. A phone's picker
// offers a handful at a time, and a message that is really a folder is a
// different feature.
const attachmentsMax = 10

// Attachment is a file hanging on a message, as everyone who reads the
// message sees it.
//
// It is a copy of what the media record said at the moment of sending, kept
// inside the message body. Drawing a page of history is then one query: no
// join, no second round trip, and a bubble that knows its own shape before
// any bytes arrive. The media record stays the authority on who may read
// the bytes; this is only what the message says it carries.
type Attachment struct {
	MediaID  uuid.UUID `json:"media_id"`
	Kind     string    `json:"kind"`
	MimeType string    `json:"mime_type"`
	ByteSize int64     `json:"byte_size"`
	FileName string    `json:"file_name"`
	// Pixels, for a picture, so the bubble is the right shape before the
	// picture loads and the list does not jump when it does.
	Width  int32 `json:"width,omitempty"`
	Height int32 `json:"height,omitempty"`
	// HasThumbnail says a small copy exists to draw in the bubble.
	HasThumbnail bool `json:"has_thumbnail,omitempty"`
	// How long a recording or video runs, and its loudness over time. Both
	// are here rather than fetched, because they are what the bubble is
	// drawn from before a single byte of audio is asked for.
	DurationMS int32   `json:"duration_ms,omitempty"`
	Waveform   []int32 `json:"waveform,omitempty"`
}

// resolveAttachments turns the ids a sender named into what the message
// will say it carries, refusing anything that is not theirs to send.
//
// This runs before the message exists, so it is the check that produces a
// good error. The claim inside the transaction is the one that is safe
// against two sends racing for the same file.
func (s *Service) resolveAttachments(ctx context.Context, sender Sender, ids []uuid.UUID) ([]Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > attachmentsMax {
		return nil, domain.InvalidField("attachments", "too_many_attachments",
			fmt.Sprintf("A message can carry at most %d files.", attachmentsMax))
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, domain.InvalidField("attachments", "invalid_attachment",
				"The same file is attached twice.")
		}
		seen[id] = true
	}

	rows, err := s.store.ListMediaByIDs(ctx, ids)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list attachments: %w", err))
	}
	found := make(map[uuid.UUID]gen.Medium, len(rows))
	for _, r := range rows {
		found[r.ID] = r
	}

	// The order the sender listed them in is the order they are shown in.
	out := make([]Attachment, 0, len(ids))
	for _, id := range ids {
		row, ok := found[id]
		if !ok || row.OwnerKind != string(sender.Kind) || row.OwnerID != sender.ID {
			// A file that is not theirs and a file that does not exist are
			// the same answer, so ids cannot be probed for.
			return nil, errUnknownAttachment()
		}
		if row.MessageID != nil {
			return nil, domain.InvalidField("attachments", "attachment_already_sent",
				"That file has already been sent. Upload it again to send it again.")
		}
		out = append(out, Attachment{
			MediaID:      row.ID,
			Kind:         row.Kind,
			MimeType:     row.MimeType,
			ByteSize:     row.ByteSize,
			FileName:     row.FileName,
			Width:        row.Width,
			Height:       row.Height,
			HasThumbnail: row.ThumbKey != nil,
			DurationMS:   row.DurationMs,
			Waveform:     row.Waveform,
		})
	}
	return out, nil
}

// claimAttachments hangs the files on the message, in the same transaction
// that created it.
//
// The row count is the whole check. Two sends racing for one file both pass
// the checks above; only one updates a row here, and the other is refused
// with the message already rolled back.
func claimAttachments(ctx context.Context, tx *store.Store, sender Sender, messageID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := tx.ClaimMedia(ctx, gen.ClaimMediaParams{
		MessageID: &messageID,
		Ids:       ids,
		OwnerKind: string(sender.Kind),
		OwnerID:   sender.ID,
	})
	if err != nil {
		return domain.Internal(fmt.Errorf("claim attachments for %s: %w", messageID, err))
	}
	if int(n) != len(ids) {
		return errUnknownAttachment()
	}
	return nil
}

func errUnknownAttachment() error {
	return domain.InvalidField("attachments", "unknown_attachment",
		"One of those files is not yours to send, or does not exist.")
}

// attachmentIDs is what the claim needs from what the body will say.
func attachmentIDs(atts []Attachment) []uuid.UUID {
	if len(atts) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(atts))
	for _, a := range atts {
		ids = append(ids, a.MediaID)
	}
	return ids
}

// previewOfAttachments is what a chat list row says about a message with no
// words: the same thing WhatsApp says, because it is the right thing —
// what arrived, not that something did.
func previewOfAttachments(atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}
	if len(atts) > 1 {
		return fmt.Sprintf("%d files", len(atts))
	}
	switch atts[0].Kind {
	case media.KindImage:
		return "Photo"
	case media.KindVideo:
		return "Video"
	case media.KindAudio:
		return "Voice note"
	default:
		return atts[0].FileName
	}
}
