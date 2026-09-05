-- The delivery outbox: what each agent's backend must be told, and whether
-- it has been.
--
-- A message is recorded and delivered in two separate steps on purpose. The
-- row here is written in the same transaction as the message, so the promise
-- "this agent must hear this" can never be lost between the two; a worker
-- keeps that promise on its own schedule, retrying for as long as it takes,
-- and the user is told the truth about whether it was kept. Doing the HTTP
-- call inside the send request instead would make every user wait on every
-- company's server, and lose messages whenever one of them was down.

-- +goose Up

CREATE TABLE message_deliveries (
    -- Also the event id a backend sees, so the identifier it de-duplicates on
    -- is the identifier we retry on.
    id              uuid        PRIMARY KEY,
    message_id      uuid        NOT NULL REFERENCES messages (id),
    agent_id        uuid        NOT NULL REFERENCES agents (id),
    event_type      text        NOT NULL,
    status          text        NOT NULL DEFAULT 'pending',
    attempts        integer     NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    delivered_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT message_deliveries_event_type_check CHECK (
        event_type IN ('message.created')
    ),
    CONSTRAINT message_deliveries_status_check CHECK (
        status IN ('pending', 'delivered', 'failed')
    ),
    -- An agent hears about a message once. Retries reuse the row.
    CONSTRAINT message_deliveries_message_agent_key UNIQUE (message_id, agent_id)
);

-- What the worker asks every second: anything due? Only pending rows are in
-- the index, so it stays small however long the history grows.
CREATE INDEX message_deliveries_due_idx
    ON message_deliveries (next_attempt_at) WHERE status = 'pending';

-- A backend catching up reads its own events in order.
CREATE INDEX message_deliveries_agent_idx ON message_deliveries (agent_id, id);

-- Five minutes of consecutive failures makes a binding unreachable. The
-- start of the current streak is the only state that rule needs.
ALTER TABLE agent_bindings ADD COLUMN failure_streak_started_at timestamptz;

-- +goose Down

ALTER TABLE agent_bindings DROP COLUMN failure_streak_started_at;
DROP TABLE message_deliveries;
