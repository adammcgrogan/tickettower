-- +goose Up
-- Private notes staff leave on a ticket, with /ticket note or from the
-- dashboard. Only the team sees them, never the member.
CREATE TABLE ticket_notes (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT      NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    author_id   BIGINT      NOT NULL,
    author_name TEXT        NOT NULL,
    content     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ticket_notes_ticket_idx ON ticket_notes (ticket_id, created_at);

-- +goose Down
DROP TABLE ticket_notes;
