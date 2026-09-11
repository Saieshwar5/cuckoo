-- Routines, seen by the person.
--
-- A routine now has a twin on the hub: the schedule the app shows, pauses and
-- deletes. The routine is still what runs — the hub fires nothing — and this
-- is how the two find each other.
ALTER TABLE routines
    -- The hub's sch_… id, once the hub knows about it.
    ADD COLUMN hub_schedule_id text UNIQUE,
    -- What the app calls it: "Morning weather".
    ADD COLUMN title           text    NOT NULL DEFAULT '',
    -- Runs once, then goes: a cron line cannot say "only this year".
    ADD COLUMN once            boolean NOT NULL DEFAULT false;
