-- Media: the bytes a message carries.
--
-- A file is uploaded before it is sent, so a slow upload does not hold a
-- message open and a failed one leaves no half-message behind. The row is
-- created by the upload, and the send claims it: message_id is null until
-- then, which is also what makes claiming a file twice impossible.
--
-- The bytes themselves are not here. This table is the record — who owns
-- them, what they are, where they went — and storage_key is where the
-- store put them, whether that is a folder on this machine or a bucket.
--
-- What a person sees in a bubble is denormalised into the message body, so
-- drawing history needs no join. This table stays the authority on who may
-- read the bytes.

-- +goose Up

CREATE TABLE media (
    id          uuid        PRIMARY KEY,
    -- Who uploaded it. Only they can attach it to a message.
    owner_kind  text        NOT NULL,
    owner_id    uuid        NOT NULL,
    -- What it is, decided by the hub from the bytes themselves rather than
    -- from what the uploader claimed.
    kind        text        NOT NULL,
    mime_type   text        NOT NULL,
    byte_size   bigint      NOT NULL,
    -- The name to save it under. Cleaned on upload: one path segment, never
    -- a path.
    file_name   text        NOT NULL,
    -- Pixels, for a picture; 0 for anything that is not one. The app draws
    -- the bubble at the right shape before the bytes arrive.
    width       integer     NOT NULL DEFAULT 0,
    height      integer     NOT NULL DEFAULT 0,
    storage_key text        NOT NULL,
    -- The small version, for a picture. Null when there is nothing to shrink.
    thumb_key   text,
    -- Null until a message claims it.
    message_id  uuid        REFERENCES messages (id),
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT media_owner_kind_check CHECK (owner_kind IN ('user', 'agent')),
    CONSTRAINT media_kind_check CHECK (kind IN ('image', 'video', 'audio', 'file')),
    CONSTRAINT media_byte_size_check CHECK (byte_size > 0),
    CONSTRAINT media_file_name_length CHECK (char_length(file_name) BETWEEN 1 AND 255)
);

-- Reading the bytes goes through the message they hang on: one row, then a
-- membership check.
CREATE INDEX media_message_idx ON media (message_id);
-- Files a person uploaded and never sent, for the sweep that removes them.
CREATE INDEX media_unclaimed_idx ON media (created_at) WHERE message_id IS NULL;

-- +goose Down

DROP TABLE media;
