-- The runtime's own tables. Nothing here belongs to the hub, and nothing here
-- is reachable from it: the two systems share a protocol, not a database.

-- A ready-made definition. `blank` is the one custom agents are built on.
CREATE TABLE templates (
    id          text        PRIMARY KEY,
    name        text        NOT NULL,
    -- What the model is told it is. A custom agent overrides this with the
    -- person's own words.
    persona     text        NOT NULL,
    -- Names of tools in src/tools. Kept as text so adding one is a row, not
    -- a migration.
    tools       text[]      NOT NULL DEFAULT '{}',
    model       text        NOT NULL DEFAULT '',
    -- The few things it suggests saying first, shown as chips in an empty chat.
    starters    jsonb       NOT NULL DEFAULT '[]',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- One agent this runtime answers for. The identity itself lives on the hub;
-- this is what is needed to answer as it.
CREATE TABLE agents (
    id            uuid        PRIMARY KEY,
    -- The hub's own agt_… identifier. The webhook URL names it, so this is
    -- what an arriving event is looked up by.
    hub_agent_id  text        NOT NULL,
    template_id   text        NOT NULL REFERENCES templates (id),
    -- The hub's usr_… identifier of whoever owns it: Cuckoo's account for a
    -- ready-made agent, the person's own for a custom one.
    owner_user_id text        NOT NULL,
    -- Overrides the template's, when a person wrote their own.
    persona       text        NOT NULL DEFAULT '',
    model         text        NOT NULL DEFAULT '',
    -- The binding secret, encrypted. It is the whole authority to speak as
    -- this agent, so it is never at rest in the clear.
    binding_secret text       NOT NULL,
    -- A custom agent can never be handed out: the runtime refuses to mint
    -- codes for it and the app hides Share.
    private       boolean     NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz
);

CREATE UNIQUE INDEX agents_hub_id_key ON agents (hub_agent_id);
CREATE INDEX agents_owner_idx ON agents (owner_user_id) WHERE deleted_at IS NULL;

-- Everything said, in full, forever: the person's messages, the agent's
-- replies, and every tool call in between.
--
-- The hub is a ninety-day window and keeps what it needs to deliver. This is
-- the agent's memory, which is a different thing with a different lifetime,
-- and it is why a person can ask their agent about something from last year.
-- What the model is shown for one run is assembled from these rows; it is
-- never all of them.
CREATE TABLE turns (
    id              bigserial   PRIMARY KEY,
    agent_id        uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    conversation_id text        NOT NULL,
    -- 'user', 'assistant', or 'tool'.
    role            text        NOT NULL,
    text            text        NOT NULL DEFAULT '',
    -- Set on a tool row: what was called, with what, and what came back.
    tool_name       text,
    tool_args       jsonb,
    tool_result     jsonb,
    -- The hub's msg_… id, when this turn was a message there. A tool call is
    -- not, so it has none.
    hub_message_id  text,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT turns_role_known CHECK (role IN ('user', 'assistant', 'tool'))
);

-- The window is read newest-first for one conversation, which is the only way
-- it is ever read.
CREATE INDEX turns_conversation_idx ON turns (agent_id, conversation_id, id DESC);

-- Work to do, written down before it is done.
--
-- The hub gives a backend ten seconds to answer a webhook and retries anything
-- slower, but a model takes longer than that to think. So the receiver writes
-- a row, answers, and a worker picks it up: the person waits for the model
-- once, not for the model and a retry ladder.
--
-- event_id is unique, which is the whole de-duplication story: the hub
-- redelivers what it never saw acknowledged, and the second insert does
-- nothing.
CREATE TABLE jobs (
    id           bigserial   PRIMARY KEY,
    agent_id     uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    event_id     text        NOT NULL,
    type         text        NOT NULL,
    payload      jsonb       NOT NULL,
    -- 'pending', 'running', 'done', 'failed'.
    status       text        NOT NULL DEFAULT 'pending',
    attempts     integer     NOT NULL DEFAULT 0,
    run_at       timestamptz NOT NULL DEFAULT now(),
    claimed_at   timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT jobs_status_known CHECK (status IN ('pending', 'running', 'done', 'failed'))
);

CREATE UNIQUE INDEX jobs_event_key ON jobs (event_id);
-- What the worker claims: due, not finished, oldest first.
CREATE INDEX jobs_due_idx ON jobs (run_at) WHERE status IN ('pending', 'running');
