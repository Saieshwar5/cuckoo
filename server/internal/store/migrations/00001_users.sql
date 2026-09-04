-- Users: the human account behind the app.
--
-- Deliberately minimal. Login identities (email, Google, later phone) and
-- device sessions arrive with the authentication system; media and avatars
-- arrive with the media system. Each migration adds only what the code in the
-- same change actually uses, so the schema never carries speculative tables.

-- +goose Up

CREATE TABLE users (
    id           uuid        PRIMARY KEY,
    display_name text        NOT NULL,
    locale       text        NOT NULL DEFAULT 'en-IN',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    -- Soft delete: a departed user's messages must remain readable to the
    -- people and agents they talked to, so rows are retired, never removed.
    deleted_at   timestamptz,

    CONSTRAINT users_display_name_length CHECK (
        char_length(display_name) BETWEEN 1 AND 80
    ),
    CONSTRAINT users_locale_length CHECK (
        char_length(locale) BETWEEN 2 AND 16
    )
);

-- Every read of a live user goes through this predicate.
CREATE INDEX users_active_idx ON users (id) WHERE deleted_at IS NULL;

-- +goose Down

DROP TABLE users;
