-- +goose Up
-- How a ticket was closed, so analytics can tell auto-closes apart from
-- tickets whose channel was deleted (both leave closed_by NULL).
ALTER TABLE tickets ADD COLUMN auto_closed BOOLEAN NOT NULL DEFAULT false;
UPDATE tickets SET auto_closed = true
WHERE status = 'closed' AND closed_by IS NULL AND close_reason LIKE 'No activity for %';

-- Analytics filter by open and close time across both statuses.
CREATE INDEX tickets_guild_opened_idx ON tickets (guild_id, opened_at);
CREATE INDEX tickets_guild_closed_idx ON tickets (guild_id, closed_at) WHERE closed_at IS NOT NULL;

-- +goose Down
DROP INDEX tickets_guild_closed_idx;
DROP INDEX tickets_guild_opened_idx;
ALTER TABLE tickets DROP COLUMN auto_closed;
