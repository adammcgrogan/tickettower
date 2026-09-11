-- +goose Up
-- Where ticket events (opened, claimed, closed) are posted. NULL disables it.
ALTER TABLE guild_settings ADD COLUMN log_channel_id BIGINT;

-- +goose Down
ALTER TABLE guild_settings DROP COLUMN log_channel_id;
