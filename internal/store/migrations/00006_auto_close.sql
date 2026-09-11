-- +goose Up
-- Hours without activity before a ticket of this type closes itself. NULL
-- disables auto-close.
ALTER TABLE ticket_types ADD COLUMN auto_close_hours INT;

ALTER TABLE tickets
    ADD COLUMN last_activity_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- True when the opener sent the last message: the team owes a reply, so
    -- the ticket is never auto-closed.
    ADD COLUMN waiting_on_staff     BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN auto_close_warned_at TIMESTAMPTZ;

-- Backfill from existing transcripts so open tickets start with a real clock.
UPDATE tickets SET last_activity_at = opened_at;
UPDATE tickets t
SET last_activity_at = m.created_at, waiting_on_staff = (m.author_id = t.opener_id)
FROM (
    SELECT DISTINCT ON (ticket_id) ticket_id, created_at, author_id
    FROM ticket_messages WHERE NOT author_bot
    ORDER BY ticket_id, id DESC
) m
WHERE m.ticket_id = t.id;

CREATE INDEX tickets_open_activity_idx ON tickets (last_activity_at) WHERE status = 'open';

-- +goose Down
DROP INDEX tickets_open_activity_idx;
ALTER TABLE tickets
    DROP COLUMN auto_close_warned_at,
    DROP COLUMN waiting_on_staff,
    DROP COLUMN last_activity_at;
ALTER TABLE ticket_types DROP COLUMN auto_close_hours;
