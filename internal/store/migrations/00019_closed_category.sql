-- +goose Up
ALTER TABLE ticket_types
    -- Where a channel ticket goes when it closes: NULL deletes the channel
    -- after a few seconds (the original behaviour); a category keeps it
    -- there, read only, for closed_keep_days so it can be reopened.
    ADD COLUMN closed_parent_id     BIGINT,
    ADD COLUMN closed_member_access TEXT NOT NULL DEFAULT 'read'
        CHECK (closed_member_access IN ('read', 'hidden')),
    ADD COLUMN closed_keep_days     INT  NOT NULL DEFAULT 7;

ALTER TABLE tickets
    -- Set while a closed ticket's channel is being kept: when it will be
    -- deleted. NULL once it's gone (or was never kept), which is also what
    -- says whether a channel ticket can be reopened.
    ADD COLUMN channel_kept_until TIMESTAMPTZ;
CREATE INDEX tickets_kept_idx ON tickets (channel_kept_until) WHERE channel_kept_until IS NOT NULL;

-- +goose Down
DROP INDEX tickets_kept_idx;
ALTER TABLE tickets DROP COLUMN channel_kept_until;
ALTER TABLE ticket_types DROP COLUMN closed_keep_days, DROP COLUMN closed_member_access, DROP COLUMN closed_parent_id;
