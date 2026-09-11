-- +goose Up
ALTER TABLE tickets ADD COLUMN closed_by_name TEXT;

-- NULL keeps transcripts forever.
ALTER TABLE guild_settings ADD COLUMN transcript_retention_days INT;

CREATE TABLE ticket_messages (
    id            BIGINT PRIMARY KEY, -- Discord message ID
    ticket_id     BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    author_id     BIGINT      NOT NULL,
    author_name   TEXT        NOT NULL,
    author_avatar TEXT        NOT NULL DEFAULT '',
    author_bot    BOOLEAN     NOT NULL DEFAULT false,
    content       TEXT        NOT NULL DEFAULT '',
    embeds        JSONB       NOT NULL DEFAULT '[]',
    attachments   JSONB       NOT NULL DEFAULT '[]',
    created_at    TIMESTAMPTZ NOT NULL,
    edited_at     TIMESTAMPTZ,
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX ticket_messages_ticket_idx ON ticket_messages (ticket_id, id);

CREATE TABLE ticket_feedback (
    ticket_id  BIGINT PRIMARY KEY REFERENCES tickets (id) ON DELETE CASCADE,
    rating     INT         NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment    TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE ticket_feedback;
DROP TABLE ticket_messages;
ALTER TABLE guild_settings DROP COLUMN transcript_retention_days;
ALTER TABLE tickets DROP COLUMN closed_by_name;
