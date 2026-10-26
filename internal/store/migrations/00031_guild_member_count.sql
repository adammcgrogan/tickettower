-- +goose Up
-- How many members a server had when the bot last connected or joined it,
-- from Discord's guild create event. NULL until the bot has seen it since.
ALTER TABLE guilds ADD COLUMN member_count INTEGER;

-- +goose Down
ALTER TABLE guilds DROP COLUMN member_count;
