-- +goose Up
-- How many times the invite link was clicked, per day and per source (the
-- ?ref= on /api/invite), so the admin panel can show which channels bring
-- servers in. Counts only: nothing about who clicked.
CREATE TABLE invite_clicks (
    day    DATE NOT NULL,
    source TEXT NOT NULL,
    clicks INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, source)
);

-- +goose Down
DROP TABLE invite_clicks;
