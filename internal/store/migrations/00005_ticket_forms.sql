-- +goose Up
-- Questions asked in a modal when a member opens a ticket of this type.
ALTER TABLE ticket_types ADD COLUMN questions JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE ticket_types DROP COLUMN questions;
