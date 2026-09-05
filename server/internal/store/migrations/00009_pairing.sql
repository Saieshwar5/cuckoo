-- Pairing: how an agent reaches people who are not its owner.
--
-- A pair token is what sits behind a QR code or a link. Scanning it adds the
-- agent to a person's list and opens their chat with it. A token can be
-- static, on a poster, or personalised, one per customer with the company's
-- own reference in its payload. A contact is the result: a person has this
-- agent in their list, through this chat, and may have blocked it.
--
-- The outbox learns to carry events that are not about a message: the
-- agent joined a conversation, or was left. Those rows have no message and
-- carry their own small payload instead.

-- +goose Up

CREATE TABLE pair_tokens (
    id                 uuid        PRIMARY KEY,
    -- SHA-256 of the token in the link. The plaintext is shown once.
    token_hash         bytea       NOT NULL UNIQUE,
    kind               text        NOT NULL,
    agent_id           uuid        NOT NULL REFERENCES agents (id),
    -- Opaque to the hub; handed to the backend when someone joins. Null on
    -- a poster token.
    payload            jsonb,
    -- Null means unlimited: a poster. One means personalised.
    max_uses           integer,
    use_count          integer     NOT NULL DEFAULT 0,
    created_by_user_id uuid        NOT NULL REFERENCES users (id),
    expires_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    revoked_at         timestamptz,

    CONSTRAINT pair_tokens_kind_check CHECK (kind IN ('add_agent')),
    CONSTRAINT pair_tokens_max_uses_check CHECK (max_uses IS NULL OR max_uses > 0)
);

CREATE INDEX pair_tokens_agent_idx ON pair_tokens (agent_id, created_at DESC);

CREATE TABLE contacts (
    user_id            uuid        NOT NULL REFERENCES users (id),
    agent_id           uuid        NOT NULL REFERENCES agents (id),
    dm_conversation_id uuid        NOT NULL REFERENCES conversations (id),
    added_via          text        NOT NULL,
    pair_token_id      uuid        REFERENCES pair_tokens (id),
    -- Set while the person has blocked the agent: nothing moves in either
    -- direction. Cleared by adding the agent again.
    blocked_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, agent_id),
    CONSTRAINT contacts_added_via_check CHECK (added_via IN ('owner', 'pair_token'))
);

CREATE INDEX contacts_agent_idx ON contacts (agent_id);

-- Every owner already has a chat with each agent they created; that is a
-- contact too, so one table answers "who has this agent in their list".
INSERT INTO contacts (user_id, agent_id, dm_conversation_id, added_via, created_at)
SELECT a.owner_user_id, a.id, c.id, 'owner', c.created_at
FROM agents a
JOIN participants pa ON pa.agent_id = a.id
JOIN conversations c ON c.id = pa.conversation_id AND c.kind = 'dm'
JOIN participants pu ON pu.conversation_id = c.id AND pu.user_id = a.owner_user_id
ON CONFLICT DO NOTHING;

ALTER TABLE message_deliveries ALTER COLUMN message_id DROP NOT NULL;
ALTER TABLE message_deliveries ADD COLUMN conversation_id uuid REFERENCES conversations (id);
ALTER TABLE message_deliveries ADD COLUMN payload jsonb;

UPDATE message_deliveries d
SET conversation_id = m.conversation_id
FROM messages m
WHERE m.id = d.message_id;

ALTER TABLE message_deliveries ALTER COLUMN conversation_id SET NOT NULL;
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created', 'conversation.joined', 'conversation.left')
);
-- A message event names its message; every other kind has none.
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_subject_check CHECK (
    (event_type = 'message.created') = (message_id IS NOT NULL)
);

-- +goose Down

ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_subject_check;
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
DELETE FROM message_deliveries WHERE message_id IS NULL;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created')
);
ALTER TABLE message_deliveries DROP COLUMN payload;
ALTER TABLE message_deliveries DROP COLUMN conversation_id;
ALTER TABLE message_deliveries ALTER COLUMN message_id SET NOT NULL;
DROP TABLE contacts;
DROP TABLE pair_tokens;
