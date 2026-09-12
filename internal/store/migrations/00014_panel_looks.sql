-- +goose Up
-- How a ticket type looks as a button: Discord's button colour and an
-- optional label in place of the type's name.
ALTER TABLE ticket_types
    ADD COLUMN button_style TEXT NOT NULL DEFAULT 'primary'
        CHECK (button_style IN ('primary', 'secondary', 'success', 'danger')),
    ADD COLUMN button_label TEXT NOT NULL DEFAULT '';

-- Images on the ticket buttons message and the dropdown's placeholder text.
ALTER TABLE panels
    ADD COLUMN image_url     TEXT NOT NULL DEFAULT '',
    ADD COLUMN thumbnail_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN placeholder   TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE panels DROP COLUMN placeholder, DROP COLUMN thumbnail_url, DROP COLUMN image_url;
ALTER TABLE ticket_types DROP COLUMN button_label, DROP COLUMN button_style;
