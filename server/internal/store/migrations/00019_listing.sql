-- +goose Up
-- Two facts about who may reach an agent, which the pair token alone cannot
-- carry.
--
-- D29 settled that a public agent is one with a live static token: revoke it
-- and the door closes. That still holds, and neither column replaces it. They
-- answer two questions the token was never asked.
--
-- `private` says the door may not be opened at all. A person's own agent, run
-- by Cuckoo and holding their mail, is not something to hand out — so the hub
-- refuses to mint a code for it rather than trusting every caller to remember
-- (D51). Deleting the flag would make a screenshot enough to read somebody's
-- inbox.
--
-- `listed` says the agent belongs in the catalogue the app shows. Having a
-- poster code and wanting to appear in Cuckoo's directory are different
-- wishes: a bank hands out a QR and would be startled to find itself in a
-- list beside a weather agent. Off by default for exactly that reason. This
-- is Q6's agent directory in its first form — ours to begin with, opt-in for
-- everybody else later.
ALTER TABLE agents
    ADD COLUMN private boolean NOT NULL DEFAULT false,
    ADD COLUMN listed  boolean NOT NULL DEFAULT false;

-- An agent cannot be both hidden from everyone and offered to everyone.
ALTER TABLE agents
    ADD CONSTRAINT agents_private_not_listed CHECK (NOT (private AND listed));

-- What the catalogue asks for: everything listed, newest last so the order is
-- the order they were published in.
CREATE INDEX agents_listed_idx ON agents (created_at) WHERE listed AND deleted_at IS NULL;

-- Adding an agent from the catalogue is a third way in, beside a code and
-- owning the thing. Recorded because "how did this get here" is the first
-- question asked when somebody wants it gone.
ALTER TABLE contacts DROP CONSTRAINT contacts_added_via_check;
ALTER TABLE contacts ADD CONSTRAINT contacts_added_via_check
    CHECK (added_via IN ('owner', 'pair_token', 'hub', 'catalogue'));

-- +goose Down
ALTER TABLE contacts DROP CONSTRAINT contacts_added_via_check;
ALTER TABLE contacts ADD CONSTRAINT contacts_added_via_check
    CHECK (added_via IN ('owner', 'pair_token', 'hub'));
DROP INDEX agents_listed_idx;
ALTER TABLE agents DROP CONSTRAINT agents_private_not_listed;
ALTER TABLE agents DROP COLUMN listed, DROP COLUMN private;
