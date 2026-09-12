-- +goose Up
-- Who can open each ticket type: members need one of required_role_ids (if
-- any), can't have any of blocked_role_ids, and wait cooldown_minutes after
-- their last ticket of the type closes.
ALTER TABLE ticket_types
    ADD COLUMN required_role_ids BIGINT[] NOT NULL DEFAULT '{}',
    ADD COLUMN blocked_role_ids  BIGINT[] NOT NULL DEFAULT '{}',
    ADD COLUMN cooldown_minutes  INT      NOT NULL DEFAULT 0;

-- The cooldown looks up a member's most recently closed ticket of a type.
CREATE INDEX tickets_closed_by_user_idx ON tickets (guild_id, opener_id, ticket_type_id, closed_at DESC)
    WHERE status = 'closed';

-- Members who can't open tickets in a server at all.
CREATE TABLE ticket_blocks (
    guild_id        BIGINT      NOT NULL REFERENCES guilds (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL,
    user_name       TEXT        NOT NULL,
    reason          TEXT        NOT NULL DEFAULT '',
    blocked_by      BIGINT      NOT NULL,
    blocked_by_name TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, user_id)
);

-- +goose Down
DROP TABLE ticket_blocks;
DROP INDEX tickets_closed_by_user_idx;
ALTER TABLE ticket_types
    DROP COLUMN cooldown_minutes,
    DROP COLUMN blocked_role_ids,
    DROP COLUMN required_role_ids;
