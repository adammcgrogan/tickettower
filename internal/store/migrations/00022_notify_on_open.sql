-- +goose Up
ALTER TABLE ticket_types
    -- Who a new channel ticket pings: the support roles (default), a
    -- specific role (notify_role_id), or nobody. Thread tickets always ping
    -- the support roles, since that ping is what adds them to the thread.
    ADD COLUMN notify_on_open TEXT   NOT NULL DEFAULT 'roles'
        CHECK (notify_on_open IN ('roles', 'custom', 'none')),
    ADD COLUMN notify_role_id BIGINT;

-- +goose Down
ALTER TABLE ticket_types
    DROP COLUMN notify_role_id, DROP COLUMN notify_on_open;
