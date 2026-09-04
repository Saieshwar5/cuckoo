-- Agents and their bindings.
--
-- An agent is an identity: a name in a chat list, owned by a person. A binding
-- is the connection between that identity and whatever backend answers for it.
-- They are separate tables because they have different lifetimes: a company
-- revokes a binding and the user keeps their chat history; a user re-points
-- their own agent at a different provider and nothing else changes. Everything
-- that accumulates — conversations, messages — hangs off the agent, never the
-- binding.

-- +goose Up

CREATE TABLE agents (
    id            uuid        PRIMARY KEY,
    owner_user_id uuid        NOT NULL REFERENCES users (id),
    -- Stable, human-typeable name. Forms the internal identifier
    -- @handle:hub-domain, so it is unique for the life of the hub: a deleted
    -- agent's handle is retired, never reissued to a different identity.
    handle        text        NOT NULL,
    display_name  text        NOT NULL,
    description   text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz,

    CONSTRAINT agents_handle_format CHECK (
        handle ~ '^[a-z0-9][a-z0-9_-]{2,31}$'
    ),
    CONSTRAINT agents_display_name_length CHECK (
        char_length(display_name) BETWEEN 1 AND 80
    ),
    CONSTRAINT agents_description_length CHECK (
        char_length(description) <= 500
    )
);

CREATE UNIQUE INDEX agents_handle_key ON agents (handle);
CREATE INDEX agents_owner_idx ON agents (owner_user_id) WHERE deleted_at IS NULL;

CREATE TABLE agent_bindings (
    id           uuid        PRIMARY KEY,
    agent_id     uuid        NOT NULL REFERENCES agents (id),
    -- How the backend receives events. A webhook needs a URL we call; a
    -- socket backend connects to us and needs nothing but the secret.
    mode         text        NOT NULL,
    webhook_url  text,
    -- SHA-256 of the bearer secret. The plaintext is shown once at creation
    -- and never stored. A fast hash is correct here — the secret is 256 random
    -- bits, so it cannot be guessed however cheap the hash is — and the lookup
    -- runs on every request an agent backend makes.
    secret_hash  bytea       NOT NULL,
    -- Operational state. Revocation is the separate revoked_at column, so a
    -- binding's last known health survives being revoked.
    status       text        NOT NULL DEFAULT 'idle',
    last_seen_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,

    CONSTRAINT agent_bindings_mode_check CHECK (
        mode IN ('webhook', 'socket')
    ),
    CONSTRAINT agent_bindings_status_check CHECK (
        status IN ('idle', 'connected', 'unreachable')
    ),
    -- A webhook binding has a URL and a socket binding does not; anything
    -- else is a bug the database refuses to store.
    CONSTRAINT agent_bindings_webhook_url_check CHECK (
        (mode = 'webhook') = (webhook_url IS NOT NULL)
    ),
    CONSTRAINT agent_bindings_secret_hash_length CHECK (
        octet_length(secret_hash) = 32
    )
);

-- An agent has at most one live binding. Setting a new one revokes the old
-- one in the same transaction; this index makes any other sequence impossible.
CREATE UNIQUE INDEX agent_bindings_active_key
    ON agent_bindings (agent_id) WHERE revoked_at IS NULL;

-- Authentication looks a secret up by its hash on every agent request.
CREATE UNIQUE INDEX agent_bindings_secret_hash_key ON agent_bindings (secret_hash);

-- +goose Down

DROP TABLE agent_bindings;
DROP TABLE agents;
