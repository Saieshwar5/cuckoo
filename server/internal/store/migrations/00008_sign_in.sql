-- Sign-in: who a person is, how they proved it, and the devices they hold.
--
-- An identity is one way to log in. Email is the only kind today; a second
-- kind is a new row with a new value, not a migration. A code is one attempt
-- to prove an email, short-lived and single use. A session is one signed-in
-- device, holding the hash of the token it presents on every request.

-- +goose Up

CREATE TABLE user_identities (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id),
    kind        text        NOT NULL,
    value       text        NOT NULL,
    verified_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT user_identities_kind_check CHECK (kind IN ('email')),
    CONSTRAINT user_identities_value_length CHECK (char_length(value) BETWEEN 3 AND 320)
);

-- One account per address.
CREATE UNIQUE INDEX user_identities_kind_value_key ON user_identities (kind, value);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

CREATE TABLE sign_in_codes (
    id         uuid        PRIMARY KEY,
    email      text        NOT NULL,
    -- SHA-256 of the code, salted with the row id. The code space is small,
    -- so the real protection is the attempt limit; hashing keeps codes out
    -- of the database and its backups all the same.
    code_hash  bytea       NOT NULL,
    attempts   integer     NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sign_in_codes_hash_length CHECK (octet_length(code_hash) = 32)
);

-- Verification looks up the newest code for an address.
CREATE INDEX sign_in_codes_email_idx ON sign_in_codes (email, created_at DESC);

CREATE TABLE sessions (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id),
    -- SHA-256 of the bearer token, which is 256 random bits: the same
    -- reasoning as binding secrets, and the same one indexed lookup per
    -- request.
    token_hash  bytea       NOT NULL,
    device_name text        NOT NULL DEFAULT '',
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,

    CONSTRAINT sessions_hash_length CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_device_name_length CHECK (char_length(device_name) <= 80)
);

CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
-- "Sign out the other devices" finds a person's live sessions here.
CREATE INDEX sessions_user_live_idx ON sessions (user_id) WHERE revoked_at IS NULL;

-- +goose Down

DROP TABLE sessions;
DROP TABLE sign_in_codes;
DROP TABLE user_identities;
