-- +goose Up
-- What a person may do about what they have seen: put it out of their own
-- sight, and tell the hub's operator about it.

-- cleared_before: the person cleared the chat. Everything up to and
-- including this message is out of their view; the agent, and anyone else
-- in the conversation, still has it. Only user rows carry a value.
ALTER TABLE participants ADD COLUMN cleared_before uuid REFERENCES messages (id);

-- One person, one message they no longer want to see. Nobody else notices.
CREATE TABLE message_hides (
    user_id    uuid        NOT NULL REFERENCES users (id),
    message_id uuid        NOT NULL REFERENCES messages (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, message_id)
);

-- A report is read by a person, not a process. The queue is this table.
CREATE TABLE reports (
    id               uuid        PRIMARY KEY,
    reporter_user_id uuid        NOT NULL REFERENCES users (id),
    agent_id         uuid        NOT NULL REFERENCES agents (id),
    message_id       uuid        REFERENCES messages (id),
    reason           text        NOT NULL,
    note             text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reports_reason_check CHECK (reason IN ('spam', 'impersonation', 'abuse', 'other')),
    CONSTRAINT reports_note_length CHECK (char_length(note) <= 500)
);

CREATE INDEX reports_reporter_agent_idx ON reports (reporter_user_id, agent_id, created_at DESC);

-- +goose Down
DROP TABLE reports;
DROP TABLE message_hides;
ALTER TABLE participants DROP COLUMN cleared_before;
