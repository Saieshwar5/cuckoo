-- +goose Up
-- Two things a person can now do to a conversation: stop an agent mid-reply,
-- and have read it.

-- A reply the person stopped. It is also truncated — its text is not what
-- the agent meant to finish, and truncated is what the hub's signature
-- covers — and stopped says why: the person asked, rather than the agent
-- going quiet. Kept outside the signature on purpose, since adding a field
-- to what is signed would unmake every signature so far.
ALTER TABLE messages ADD COLUMN stopped boolean NOT NULL DEFAULT false;

-- How far a person has read: the newest message they have seen. A position,
-- not a reference — ids are ordered in time, so "after this id" still means
-- something once retention has swept the message itself, the same reasoning
-- cleared_before follows. Null for an agent, which has no screen.
ALTER TABLE participants ADD COLUMN read_up_to uuid;

-- Everything said before today counts as read. The alternative is every
-- existing chat lighting up with a count of its whole history.
UPDATE participants p
SET read_up_to = (SELECT m.id FROM messages m WHERE m.conversation_id = p.conversation_id ORDER BY m.id DESC LIMIT 1)
WHERE p.user_id IS NOT NULL;

-- The agent is told a person pressed stop, the same way it is told
-- everything else: an outbox row, retried until its backend has it.
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created', 'conversation.joined', 'conversation.left', 'stop.requested')
);

-- +goose Down
DELETE FROM message_deliveries WHERE event_type = 'stop.requested';
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created', 'conversation.joined', 'conversation.left')
);
ALTER TABLE participants DROP COLUMN read_up_to;
ALTER TABLE messages DROP COLUMN stopped;
