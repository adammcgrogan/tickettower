-- +goose Up
-- When a ticket's current claim started, for the "time to claim" analytics
-- tile. Cleared when unclaimed, like claimed_by, so it always reflects the
-- ticket's current (or most recent) claim.
ALTER TABLE tickets ADD COLUMN claimed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE tickets DROP COLUMN claimed_at;
