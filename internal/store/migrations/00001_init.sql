-- +goose Up
CREATE TABLE guilds (
    id         BIGINT PRIMARY KEY,
    name       TEXT        NOT NULL,
    icon       TEXT,
    owner_id   BIGINT      NOT NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at    TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE guild_settings (
    guild_id           BIGINT PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    dashboard_role_ids BIGINT[]    NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Premium-ready: every guild is implicitly on the free tier unless a row
-- here says otherwise.
CREATE TABLE entitlements (
    guild_id   BIGINT PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    tier       TEXT        NOT NULL DEFAULT 'free',
    source     TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE entitlements;
DROP TABLE guild_settings;
DROP TABLE guilds;
