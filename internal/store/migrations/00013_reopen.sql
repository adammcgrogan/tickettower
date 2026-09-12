-- +goose Up
-- When a closed ticket was last reopened, for the log and so a reopened
-- ticket's history reads right.
ALTER TABLE tickets ADD COLUMN reopened_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE tickets DROP COLUMN reopened_at;
