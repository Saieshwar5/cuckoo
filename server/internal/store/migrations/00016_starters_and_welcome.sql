-- +goose Up
-- starters: a few things an agent suggests saying first, shown as chips in
-- an empty chat. The agent's owner sets them; tapping one sends its words.
-- A company's agent without them opens on a blank screen and a person who
-- does not know what to type.
ALTER TABLE agents ADD COLUMN starters jsonb NOT NULL DEFAULT '[]'::jsonb;

-- A contact the hub itself made: the welcome agent, given to every new
-- account so the first screen is never empty.
ALTER TABLE contacts DROP CONSTRAINT contacts_added_via_check;
ALTER TABLE contacts ADD CONSTRAINT contacts_added_via_check
    CHECK (added_via IN ('owner', 'pair_token', 'hub'));

-- +goose Down
ALTER TABLE contacts DROP CONSTRAINT contacts_added_via_check;
ALTER TABLE contacts ADD CONSTRAINT contacts_added_via_check
    CHECK (added_via IN ('owner', 'pair_token'));
ALTER TABLE agents DROP COLUMN starters;
