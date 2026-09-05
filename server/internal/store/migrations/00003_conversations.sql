-- Conversations, who is in them, and what was said.
--
-- The model is participant-based from the first migration: a conversation has
-- members, each either a person or an agent, and a message has a sender of
-- either kind. Today the only conversation ever created is the DM between an
-- agent and its owner, and only people send. Groups, agents replying, and
-- agents talking to each other inside a team all fit this shape unchanged,
-- which is the point of choosing it now rather than modelling "human writes
-- to bot" and migrating later.

-- +goose Up

CREATE TABLE conversations (
    id         uuid        PRIMARY KEY,
    kind       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT conversations_kind_check CHECK (kind IN ('dm', 'group'))
);

CREATE TABLE participants (
    conversation_id uuid        NOT NULL REFERENCES conversations (id),
    kind            text        NOT NULL,
    user_id         uuid        REFERENCES users (id),
    agent_id        uuid        REFERENCES agents (id),
    joined_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT participants_kind_check CHECK (kind IN ('user', 'agent')),
    -- A participant is exactly one of a person or an agent, and kind says
    -- which, so a row can be neither ambiguous nor empty.
    CONSTRAINT participants_identity_check CHECK (
        (kind = 'user')  = (user_id  IS NOT NULL) AND
        (kind = 'agent') = (agent_id IS NOT NULL)
    )
);

-- Each person and each agent is in a conversation at most once.
CREATE UNIQUE INDEX participants_user_key
    ON participants (conversation_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX participants_agent_key
    ON participants (conversation_id, agent_id) WHERE agent_id IS NOT NULL;

-- "Which conversations am I in" is the chat list, the app's first screen.
CREATE INDEX participants_user_idx ON participants (user_id) WHERE user_id IS NOT NULL;

CREATE TABLE messages (
    id              uuid        PRIMARY KEY,
    conversation_id uuid        NOT NULL REFERENCES conversations (id),
    sender_kind     text        NOT NULL,
    sender_user_id  uuid        REFERENCES users (id),
    sender_agent_id uuid        REFERENCES agents (id),
    -- The content, as one JSON object. Text today; attachments, buttons and
    -- quick replies join it without a migration, because the protocol's
    -- message body is a single object and growing it is the normal case.
    body            jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_sender_kind_check CHECK (sender_kind IN ('user', 'agent')),
    CONSTRAINT messages_sender_check CHECK (
        (sender_kind = 'user')  = (sender_user_id  IS NOT NULL) AND
        (sender_kind = 'agent') = (sender_agent_id IS NOT NULL)
    ),
    CONSTRAINT messages_body_is_object CHECK (jsonb_typeof(body) = 'object')
);

-- Identifiers are UUIDv7, so ordering by id is ordering by creation time.
-- History pages newest-first on this index alone, with no timestamp column
-- in the cursor.
CREATE INDEX messages_conversation_idx ON messages (conversation_id, id DESC);

-- +goose Down

DROP TABLE messages;
DROP TABLE participants;
DROP TABLE conversations;
