-- +goose Up
-- Several signed-in devices per person, and a way to tell which is which.
--
-- Until now signing in anywhere signed out everywhere else, enforced in the
-- service rather than the schema, so this is a rule change and one column.

-- When this session last carried a request. Written at most once an hour, so
-- the list can say "last seen yesterday" without a write per request. Null
-- means it has not been used since it was created.
ALTER TABLE sessions ADD COLUMN last_seen_at timestamptz;

-- The device list, newest first, and the sweep that ends the oldest when
-- somebody passes the limit.
CREATE INDEX sessions_user_live_idx
    ON sessions (user_id, created_at DESC) WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX sessions_user_live_idx;
ALTER TABLE sessions DROP COLUMN last_seen_at;
