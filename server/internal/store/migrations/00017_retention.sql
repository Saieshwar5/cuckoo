-- +goose Up
-- The hub keeps a window of history, not an archive: messages and their
-- files go after a period, and a person's files are held to a budget. What
-- points at a message must either survive it going or go with it.

-- A reply keeps the id of what it answered after the original has expired.
-- The quote is drawn from the original while it is there and simply not
-- drawn afterwards; the id stays so an archive handed back later can be
-- matched up.
ALTER TABLE messages DROP CONSTRAINT messages_reply_to_message_id_fkey;

-- What a person cleared or hid stays known after the messages themselves
-- are gone, so nothing handed back later can put it in front of them again.
-- Ids are ordered in time, so "before this id" keeps working without the row.
ALTER TABLE participants DROP CONSTRAINT participants_cleared_before_fkey;
ALTER TABLE message_hides DROP CONSTRAINT message_hides_message_id_fkey;

-- A report keeps naming the message it was about.
ALTER TABLE reports DROP CONSTRAINT reports_message_id_fkey;

-- Delivery rows are bookkeeping for a message and go with it.
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_message_id_fkey;
ALTER TABLE message_deliveries
    ADD CONSTRAINT message_deliveries_message_id_fkey
    FOREIGN KEY (message_id) REFERENCES messages (id) ON DELETE CASCADE;

-- The hub's own signature over a finished message's content, so a copy kept
-- elsewhere can be shown to be exactly what was said. Null while a stream
-- is still being written, and on messages from before signing existed.
ALTER TABLE messages ADD COLUMN signature bytea;

-- What the sweeper asks: anything older than the window?
CREATE INDEX messages_created_idx ON messages (created_at);
-- And: whose files add up to more than their budget, oldest first?
CREATE INDEX media_owner_claimed_idx
    ON media (owner_kind, owner_id, created_at) WHERE message_id IS NOT NULL;

-- +goose Down
DROP INDEX media_owner_claimed_idx;
DROP INDEX messages_created_idx;
ALTER TABLE messages DROP COLUMN signature;
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_message_id_fkey;
ALTER TABLE message_deliveries
    ADD CONSTRAINT message_deliveries_message_id_fkey
    FOREIGN KEY (message_id) REFERENCES messages (id);
ALTER TABLE reports ADD CONSTRAINT reports_message_id_fkey
    FOREIGN KEY (message_id) REFERENCES messages (id);
ALTER TABLE message_hides ADD CONSTRAINT message_hides_message_id_fkey
    FOREIGN KEY (message_id) REFERENCES messages (id);
ALTER TABLE participants ADD CONSTRAINT participants_cleared_before_fkey
    FOREIGN KEY (cleared_before) REFERENCES messages (id);
ALTER TABLE messages ADD CONSTRAINT messages_reply_to_message_id_fkey
    FOREIGN KEY (reply_to_message_id) REFERENCES messages (id);
