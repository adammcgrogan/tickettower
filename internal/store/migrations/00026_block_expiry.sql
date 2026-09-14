-- +goose Up
-- Blocks can now be temporary: NULL means it never expires.
ALTER TABLE ticket_blocks ADD COLUMN expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE ticket_blocks DROP COLUMN expires_at;
