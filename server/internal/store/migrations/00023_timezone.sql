-- +goose Up
-- Where a person's clock is: the time zone their phone is set to, as their
-- phone last reported it. "Every morning at 7" means their seven, so the
-- agents they talk to are told it, and their schedules follow it when they
-- travel. Empty until a phone has said.
ALTER TABLE users ADD COLUMN timezone text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN timezone;
