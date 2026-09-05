-- Idempotency keys on messages.
--
-- A sender that never hears back from a send cannot tell whether it happened.
-- Mobile networks drop responses constantly, and a backend's HTTP client
-- retries on timeout, so without this the same reply appears twice. The key
-- is chosen by the sender and unique per sender; a repeat returns the
-- message it already created instead of creating another.

-- +goose Up

ALTER TABLE messages ADD COLUMN idempotency_key text;

ALTER TABLE messages ADD CONSTRAINT messages_idempotency_key_length CHECK (
    idempotency_key IS NULL OR char_length(idempotency_key) BETWEEN 1 AND 200
);

CREATE UNIQUE INDEX messages_user_idempotency_key
    ON messages (sender_user_id, idempotency_key)
    WHERE sender_user_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX messages_agent_idempotency_key
    ON messages (sender_agent_id, idempotency_key)
    WHERE sender_agent_id IS NOT NULL AND idempotency_key IS NOT NULL;

-- +goose Down

DROP INDEX messages_agent_idempotency_key;
DROP INDEX messages_user_idempotency_key;
ALTER TABLE messages DROP CONSTRAINT messages_idempotency_key_length;
ALTER TABLE messages DROP COLUMN idempotency_key;
