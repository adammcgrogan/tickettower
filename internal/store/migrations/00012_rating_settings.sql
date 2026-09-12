-- +goose Up
-- Whether a ticket type asks the opener for a rating when a ticket closes,
-- and the wording of that request ('' uses the default).
ALTER TABLE ticket_types
    ADD COLUMN ask_rating    BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN rating_prompt TEXT    NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE ticket_types DROP COLUMN rating_prompt, DROP COLUMN ask_rating;
