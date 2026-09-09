-- What each person's agents have cost, by day.
--
-- Two jobs, and the second is why it exists now rather than later. It is the
-- counter a subscription will read (D68), and it is the ceiling that stops one
-- runaway conversation from spending all night. A model that loops is not a
-- hypothetical: asked once about Goa, the first run called the weather tool
-- three times because the answers looked wrong.
--
-- Keyed by the person, not the conversation, because the bill is theirs
-- wherever they spent it.
CREATE TABLE usage (
    user_id       text        NOT NULL,
    agent_id      uuid        NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    day           date        NOT NULL,
    input_tokens  bigint      NOT NULL DEFAULT 0,
    output_tokens bigint      NOT NULL DEFAULT 0,
    runs          integer     NOT NULL DEFAULT 0,
    updated_at    timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, agent_id, day)
);

-- The one question asked before every run: what has this person spent today.
CREATE INDEX usage_person_day_idx ON usage (user_id, day);
