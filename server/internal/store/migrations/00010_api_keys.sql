-- API keys: how a company's own systems reach the management API.
--
-- Everything an owner can do from the app — create agents, connect their
-- backends, mint the codes that hand them out — a server can do with one of
-- these. It is deliberately the same set of powers through the same routes:
-- a key that could do more than its holder can do in the app would be a
-- second, weaker door into the same house.
--
-- The key belongs to an account, not to a session. It outlives sign-ins,
-- survives the phone being replaced, and ends only when it is revoked.

-- +goose Up

CREATE TABLE api_keys (
    id           uuid        PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id),
    -- What it is for, in the owner's words: "SBI website", "deploy script".
    name         text        NOT NULL,
    -- SHA-256 of the key, which is 256 random bits: the same reasoning as
    -- binding secrets and session tokens, and the same single indexed
    -- lookup per request.
    key_hash     bytea       NOT NULL,
    -- Written at most once a minute, so a forgotten key is visible without
    -- a write on every call.
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz,

    CONSTRAINT api_keys_hash_length CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_name_length CHECK (char_length(name) BETWEEN 1 AND 80)
);

CREATE UNIQUE INDEX api_keys_key_hash_key ON api_keys (key_hash);
CREATE INDEX api_keys_user_idx ON api_keys (user_id, created_at DESC);

-- +goose Down

DROP TABLE api_keys;
