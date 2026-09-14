-- +goose Up
-- Auto-assign shares out new tickets of a type round robin, among staff who
-- have opted in with /ticket available.
ALTER TABLE ticket_types ADD COLUMN auto_assign BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE available_staff (
    guild_id         BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    user_id          BIGINT      NOT NULL,
    user_name        TEXT        NOT NULL,
    last_assigned_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

-- +goose Down
DROP TABLE available_staff;
ALTER TABLE ticket_types DROP COLUMN auto_assign;
