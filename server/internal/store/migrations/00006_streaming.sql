-- Streaming: a message that is born empty, grows, and is finished once.
--
-- A model produces a paragraph over several seconds. Rather than make the
-- person wait for all of it, or litter the chat with fragments, an agent
-- starts a message, appends text as it comes, and finishes. The growing text
-- lives in Redis while it grows, because a row update per word would be the
-- most expensive way to store a sentence; the row records that the message
-- is streaming, and receives the whole text once at the end.

-- +goose Up

ALTER TABLE messages
    ADD COLUMN status    text    NOT NULL DEFAULT 'complete',
    ADD COLUMN truncated boolean NOT NULL DEFAULT false;

ALTER TABLE messages ADD CONSTRAINT messages_status_check CHECK (
    status IN ('streaming', 'complete')
);

-- A stream nobody finished is found by the sweeper here. Only streaming
-- rows are in the index, so it is empty almost all of the time.
CREATE INDEX messages_streaming_idx ON messages (created_at) WHERE status = 'streaming';

-- +goose Down

DROP INDEX messages_streaming_idx;
ALTER TABLE messages DROP CONSTRAINT messages_status_check;
ALTER TABLE messages DROP COLUMN truncated, DROP COLUMN status;
