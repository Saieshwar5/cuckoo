-- Reply-to: a message may answer an earlier one in the same conversation.
--
-- Buttons, quick replies and button taps need no schema: they are fields of
-- the message body, which is one JSON object for exactly this reason. The
-- reference to another message is a real column because it is a real
-- relationship the database should keep honest.

-- +goose Up

ALTER TABLE messages ADD COLUMN reply_to_message_id uuid REFERENCES messages (id);

-- +goose Down

ALTER TABLE messages DROP COLUMN reply_to_message_id;
