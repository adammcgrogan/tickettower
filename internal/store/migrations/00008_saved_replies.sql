-- +goose Up
-- Answers a team writes once and sends into tickets from the dashboard or
-- with /reply.
CREATE TABLE saved_replies (
    id         BIGSERIAL PRIMARY KEY,
    guild_id   BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    content    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- /reply picks a reply by name, so names are unique in a server.
CREATE UNIQUE INDEX saved_replies_guild_name_idx ON saved_replies (guild_id, lower(name));

-- +goose Down
DROP TABLE saved_replies;
