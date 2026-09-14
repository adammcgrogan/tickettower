-- +goose Up
-- The staff member behind a reply the bot posted for them (from the
-- dashboard or /reply). NULL for messages people sent themselves. Analytics
-- credit these replies to the person, not the bot.
ALTER TABLE ticket_messages ADD COLUMN sent_by BIGINT;

-- +goose Down
ALTER TABLE ticket_messages DROP COLUMN sent_by;
