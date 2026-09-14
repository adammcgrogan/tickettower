-- +goose Up
-- Lets a member list every ticket they've opened across all servers, for the
-- "my tickets" page, without scanning per guild_id.
CREATE INDEX tickets_opener_idx ON tickets (opener_id, opened_at DESC, id DESC);

-- +goose Down
DROP INDEX tickets_opener_idx;
