-- +goose Up
-- Schedules: things a person asked an agent to do at a time, again and again.
--
-- The agent runs them, not the hub. Its backend owns the timer, the work and
-- the answer; the hub never fires anything. What lives here is the copy a
-- person's phone shows — what it does, when, whether it is paused — so the
-- list opens at once and still opens when the agent's backend is down, and
-- the person's word on it is final: a schedule they paused or deleted here
-- can no longer speak, whatever the backend thinks.

-- An agent says it can hold schedules; the app offers them only then.
ALTER TABLE agents ADD COLUMN supports_schedules boolean NOT NULL DEFAULT false;

CREATE TABLE schedules (
    id              uuid        PRIMARY KEY,
    agent_id        uuid        NOT NULL REFERENCES agents (id),
    conversation_id uuid        NOT NULL REFERENCES conversations (id),
    -- What the agent calls it: "Morning weather". A person's request starts
    -- with their own words here until the agent names it.
    title           text        NOT NULL,
    -- What to do, in the person's words.
    instruction     text        NOT NULL,
    -- When, as structure rather than a sentence, so "7" is never read as
    -- seven in the evening: {"repeat", "time", "days", "date", "timezone"}.
    cadence         jsonb       NOT NULL,
    -- pending: asked for, the agent has not confirmed. active. paused.
    status          text        NOT NULL,
    created_by      text        NOT NULL,
    -- The last time a message came from it, as the hub saw it land.
    last_run_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- Kept, not removed: a message the backend sends for it afterwards is
    -- refused, and the refusal needs the row to say why.
    deleted_at      timestamptz,

    CONSTRAINT schedules_status_check CHECK (status IN ('pending', 'active', 'paused')),
    CONSTRAINT schedules_created_by_check CHECK (created_by IN ('user', 'agent')),
    CONSTRAINT schedules_cadence_is_object CHECK (jsonb_typeof(cadence) = 'object')
);

-- A conversation's list, the only way they are ever read.
CREATE INDEX schedules_conversation_idx ON schedules (conversation_id, created_at) WHERE deleted_at IS NULL;

-- A message sent by a schedule says which, so the person can see why the
-- agent spoke first.
ALTER TABLE messages ADD COLUMN schedule_id uuid REFERENCES schedules (id);

-- The agent hears what the person did to a schedule through the outbox,
-- like everything else.
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created', 'conversation.joined', 'conversation.left', 'stop.requested',
                   'schedule.requested', 'schedule.updated', 'schedule.deleted')
);

-- +goose Down
DELETE FROM message_deliveries WHERE event_type LIKE 'schedule.%';
ALTER TABLE message_deliveries DROP CONSTRAINT message_deliveries_event_type_check;
ALTER TABLE message_deliveries ADD CONSTRAINT message_deliveries_event_type_check CHECK (
    event_type IN ('message.created', 'conversation.joined', 'conversation.left', 'stop.requested')
);
ALTER TABLE messages DROP COLUMN schedule_id;
DROP TABLE schedules;
ALTER TABLE agents DROP COLUMN supports_schedules;
