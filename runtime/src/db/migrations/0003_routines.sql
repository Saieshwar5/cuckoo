-- Speaking first: the three tables it takes for an agent to say something
-- nobody asked for, at a time nobody was watching.

-- A standing instruction and when to run it.
--
-- "Every morning at 8, summarise my inbox" is a row here. The instruction is
-- the person's own words, run as though they had just said them, which is why
-- there is nothing here but a schedule and a sentence: a routine is not a
-- second kind of conversation, it is the same conversation on a timer.
CREATE TABLE routines (
    id              uuid        PRIMARY KEY,
    agent_id        uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    conversation_id text        NOT NULL,
    -- Whose it is, for the proactive budget and for deleting an account.
    user_id         text        NOT NULL,
    instruction     text        NOT NULL,
    -- Five fields, standard cron. The zone matters as much as the time:
    -- "eight in the morning" is a claim about where somebody is standing.
    schedule        text        NOT NULL,
    timezone        text        NOT NULL DEFAULT 'Asia/Kolkata',
    next_run        timestamptz NOT NULL,
    last_run        timestamptz,
    -- A routine that keeps failing is paused rather than retried forever, and
    -- the person is told once. Silence and a stopped routine are the same
    -- thing to them unless somebody says so.
    failures        integer     NOT NULL DEFAULT 0,
    paused_at       timestamptz,
    paused_reason   text,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT routines_instruction_length CHECK (char_length(instruction) BETWEEN 1 AND 2000)
);

-- What the scheduler asks for: due, not paused, oldest first.
CREATE INDEX routines_due_idx ON routines (next_run) WHERE paused_at IS NULL;
CREATE INDEX routines_agent_idx ON routines (agent_id, conversation_id);

-- What a button will do when it is tapped.
--
-- Nothing that changes the world happens without one of these: the agent
-- offers, the person taps, and the tap is looked up here and executed. The row
-- is the record of what was agreed to, which is the audit trail — and it is
-- why the offer can be trusted even when the model that wrote it cannot be.
CREATE TABLE pending_actions (
    -- The button id the agent offered; it comes back on the tap.
    button_id       text        PRIMARY KEY,
    agent_id        uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    conversation_id text        NOT NULL,
    user_id         text        NOT NULL,
    -- What to do, and with what. Named rather than freeform: a tap can only
    -- ever run something this runtime already knows how to do.
    action          text        NOT NULL,
    arguments       jsonb       NOT NULL DEFAULT '{}',
    expires_at      timestamptz NOT NULL,
    taken_at        timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pending_actions_expiry_idx ON pending_actions (expires_at) WHERE taken_at IS NULL;

-- Every message sent that nobody asked for.
--
-- The hub allows an agent to speak first and counts nothing, so the count
-- lives here. Ten a day per person (Q12) is not bureaucracy: a bug in a
-- scheduler is somebody's phone at three in the morning, and the only remedy
-- they have is blocking the agent, which they will do once and for good.
CREATE TABLE proactive_sends (
    id              bigserial   PRIMARY KEY,
    user_id         text        NOT NULL,
    agent_id        uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    conversation_id text        NOT NULL,
    reason          text        NOT NULL,
    sent_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX proactive_sends_person_day_idx ON proactive_sends (user_id, sent_at DESC);
