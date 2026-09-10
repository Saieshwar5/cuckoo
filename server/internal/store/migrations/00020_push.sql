-- +goose Up
-- Where a device can be reached when nobody is looking at it.
--
-- On the session rather than in a table of its own, because a push token is
-- exactly what a session already is: one install, on one phone, belonging to
-- one person. Signing out a lost handset (D58) then stops its notifications
-- for free, with nothing else to remember — and there is no way to end a
-- session but leave it able to buzz.
ALTER TABLE sessions
    -- An Expo token, "ExponentPushToken[…]". Opaque here: the hub hands it
    -- back to Expo and never reads inside it.
    ADD COLUMN push_token text,
    -- Where it came from, for the day iOS and Android need telling apart.
    ADD COLUMN push_platform text NOT NULL DEFAULT '',
    ADD COLUMN push_registered_at timestamptz;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_push_token_length CHECK (push_token IS NULL OR char_length(push_token) BETWEEN 1 AND 256),
    ADD CONSTRAINT sessions_push_platform_known CHECK (push_platform IN ('', 'android', 'ios'));

-- What sending asks for: the live devices of one person that can be reached.
-- Partial, because most of this table has no token and never will — a browser
-- tab cannot be woken.
CREATE INDEX sessions_push_idx ON sessions (user_id)
    WHERE push_token IS NOT NULL AND revoked_at IS NULL;

-- The same token can only belong to one session at a time. Reinstalling the
-- app, or signing in as somebody else on the same phone, issues the same token
-- to a new session, and the old one must stop being reachable — otherwise one
-- phone gets another person's notifications, which is the worst version of
-- this feature failing.
CREATE UNIQUE INDEX sessions_push_token_key ON sessions (push_token)
    WHERE push_token IS NOT NULL;

-- +goose Down
DROP INDEX sessions_push_token_key;
DROP INDEX sessions_push_idx;
ALTER TABLE sessions
    DROP CONSTRAINT sessions_push_platform_known,
    DROP CONSTRAINT sessions_push_token_length;
ALTER TABLE sessions
    DROP COLUMN push_registered_at, DROP COLUMN push_platform, DROP COLUMN push_token;
