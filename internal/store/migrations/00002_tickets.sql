-- +goose Up
ALTER TABLE guild_settings ADD COLUMN ticket_counter INT NOT NULL DEFAULT 0;

CREATE TABLE ticket_types (
    id                BIGSERIAL PRIMARY KEY,
    guild_id          BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    emoji             TEXT        NOT NULL DEFAULT '',
    description       TEXT        NOT NULL DEFAULT '',
    mode              TEXT        NOT NULL DEFAULT 'channel' CHECK (mode IN ('channel', 'thread')),
    -- Category for channel mode (optional), text channel for thread mode.
    parent_id         BIGINT,
    support_role_ids  BIGINT[]    NOT NULL DEFAULT '{}',
    name_format       TEXT        NOT NULL DEFAULT 'ticket-{number}',
    welcome_message   TEXT        NOT NULL DEFAULT '',
    max_open_per_user INT         NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ticket_types_guild_idx ON ticket_types (guild_id);

CREATE TABLE panels (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    color       INT         NOT NULL DEFAULT 8154367,
    style       TEXT        NOT NULL DEFAULT 'buttons' CHECK (style IN ('buttons', 'dropdown')),
    channel_id  BIGINT,
    message_id  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX panels_guild_idx ON panels (guild_id);

CREATE TABLE panel_ticket_types (
    panel_id       BIGINT NOT NULL REFERENCES panels (id) ON DELETE CASCADE,
    ticket_type_id BIGINT NOT NULL REFERENCES ticket_types (id) ON DELETE CASCADE,
    position       INT    NOT NULL,
    PRIMARY KEY (panel_id, ticket_type_id)
);

CREATE TABLE tickets (
    id                BIGSERIAL PRIMARY KEY,
    guild_id          BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    number            INT         NOT NULL,
    ticket_type_id    BIGINT      REFERENCES ticket_types (id) ON DELETE SET NULL,
    -- Snapshot so history still reads well after the type is renamed or deleted.
    type_name         TEXT        NOT NULL,
    mode              TEXT        NOT NULL CHECK (mode IN ('channel', 'thread')),
    channel_id        BIGINT      NOT NULL,
    opener_id         BIGINT      NOT NULL,
    opener_name       TEXT        NOT NULL,
    claimed_by        BIGINT,
    claimed_by_name   TEXT,
    status            TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    close_reason      TEXT        NOT NULL DEFAULT '',
    closed_by         BIGINT,
    opened_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_response_at TIMESTAMPTZ,
    closed_at         TIMESTAMPTZ,
    UNIQUE (guild_id, number)
);
CREATE UNIQUE INDEX tickets_channel_idx ON tickets (channel_id);
CREATE INDEX tickets_guild_status_idx ON tickets (guild_id, status, opened_at DESC);
CREATE INDEX tickets_open_by_user_idx ON tickets (guild_id, opener_id) WHERE status = 'open';

-- +goose Down
DROP TABLE tickets;
DROP TABLE panel_ticket_types;
DROP TABLE panels;
DROP TABLE ticket_types;
ALTER TABLE guild_settings DROP COLUMN ticket_counter;
