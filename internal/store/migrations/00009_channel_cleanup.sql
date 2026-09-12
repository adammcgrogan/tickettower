-- +goose Up
-- When the bot finished with a closed ticket's channel: deleted it, archived
-- the thread, or found it already gone. NULL on a closed ticket means the
-- cleanup is still owed (the bot restarted mid-close, or Discord refused),
-- and the bot retries it.
ALTER TABLE tickets ADD COLUMN channel_cleaned_at TIMESTAMPTZ;
UPDATE tickets SET channel_cleaned_at = closed_at WHERE status = 'closed';
CREATE INDEX tickets_cleanup_idx ON tickets (closed_at) WHERE status = 'closed' AND channel_cleaned_at IS NULL;

-- +goose Down
DROP INDEX tickets_cleanup_idx;
ALTER TABLE tickets DROP COLUMN channel_cleaned_at;
