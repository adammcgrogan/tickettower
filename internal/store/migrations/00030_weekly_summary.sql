-- +goose Up
-- A premium server can opt in to a weekly summary of how support went,
-- posted to its log channel. sent_at is recorded before the message posts,
-- so a restart never posts it twice.
ALTER TABLE guild_settings ADD COLUMN weekly_summary BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE guild_settings ADD COLUMN weekly_summary_sent_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE guild_settings DROP COLUMN weekly_summary_sent_at;
ALTER TABLE guild_settings DROP COLUMN weekly_summary;
