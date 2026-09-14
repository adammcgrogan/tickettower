-- +goose Up
-- Suggested answers shown to a member before a ticket of this type opens,
-- so common questions can be answered without one.
ALTER TABLE ticket_types ADD COLUMN answers JSONB NOT NULL DEFAULT '[]';

-- Logged each time a member says an answer solved it instead of opening a
-- ticket, for Analytics. ticket_type_id is nullable so a deleted type
-- doesn't take its history with it.
CREATE TABLE answer_deflections (
    id             BIGSERIAL PRIMARY KEY,
    guild_id       BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    ticket_type_id BIGINT      REFERENCES ticket_types (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX answer_deflections_guild_idx ON answer_deflections (guild_id, created_at);

-- +goose Down
DROP TABLE answer_deflections;
ALTER TABLE ticket_types DROP COLUMN answers;
