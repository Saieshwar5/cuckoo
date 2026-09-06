-- +goose Up
-- What a person decides about an agent in their list, beyond blocking it.
-- These are the person's own settings, so they live on the contact row, not
-- the agent: two people can have the same agent pinned and muted differently.
--
-- muted_until: while in the future, nothing about this agent may disturb
--   the person — no notification, no count on the icon. Messages still
--   arrive and are read by opening the chat. Far in the future means always.
-- pinned_at: the chat sits above the others, in pin order.
-- archived_at: the chat is out of the main list and under Archived.
-- removed_at: the person took the agent out of their list. History stays
--   readable; the agent is not told; scanning its code again restores it,
--   with these settings cleared. Only an added agent can be removed — an
--   owner deletes theirs instead.
ALTER TABLE contacts
    ADD COLUMN muted_until timestamptz,
    ADD COLUMN pinned_at   timestamptz,
    ADD COLUMN archived_at timestamptz,
    ADD COLUMN removed_at  timestamptz;

-- +goose Down
ALTER TABLE contacts
    DROP COLUMN muted_until,
    DROP COLUMN pinned_at,
    DROP COLUMN archived_at,
    DROP COLUMN removed_at;
