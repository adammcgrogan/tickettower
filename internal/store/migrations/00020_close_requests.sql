-- +goose Up
-- A close request: staff asked the member to confirm the ticket is
-- resolved. close_request_by is set while one is pending; closes_at is when
-- the ticket closes on its own if the member doesn't answer (NULL: never).
ALTER TABLE tickets
    ADD COLUMN close_request_by        BIGINT,
    ADD COLUMN close_request_by_name   TEXT        NOT NULL DEFAULT '',
    ADD COLUMN close_request_reason    TEXT        NOT NULL DEFAULT '',
    ADD COLUMN close_request_closes_at TIMESTAMPTZ;
CREATE INDEX tickets_close_request_idx ON tickets (close_request_closes_at)
    WHERE status = 'open' AND close_request_closes_at IS NOT NULL;

-- +goose Down
DROP INDEX tickets_close_request_idx;
ALTER TABLE tickets
    DROP COLUMN close_request_closes_at, DROP COLUMN close_request_reason,
    DROP COLUMN close_request_by_name, DROP COLUMN close_request_by;
