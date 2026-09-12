-- +goose Up
ALTER TABLE ticket_types
    -- What happens to the rest of the support team when a ticket is claimed
    -- (channel tickets only): nothing, they can read but not write, or they
    -- can't see it. Roles in claim_lock_exempt_role_ids keep full access.
    ADD COLUMN claim_lock                 TEXT     NOT NULL DEFAULT 'off'
        CHECK (claim_lock IN ('off', 'read_only', 'hidden')),
    ADD COLUMN claim_lock_exempt_role_ids BIGINT[] NOT NULL DEFAULT '{}',
    -- How quickly the team aims to reply (the target shown in Analytics),
    -- and when to remind them about a ticket waiting on the team.
    ADD COLUMN reply_target_minutes       INT,
    ADD COLUMN reminder_minutes           INT,
    ADD COLUMN reminder_repeat            BOOLEAN  NOT NULL DEFAULT false,
    ADD COLUMN reminder_where             TEXT     NOT NULL DEFAULT 'ticket'
        CHECK (reminder_where IN ('ticket', 'log', 'both')),
    ADD COLUMN reminder_ping              TEXT     NOT NULL DEFAULT 'claimer'
        CHECK (reminder_ping IN ('claimer', 'roles', 'none'));

ALTER TABLE tickets
    -- On hold: waiting on something other than the member or the team. Left
    -- out of the team's queue, reminders and auto-close until resumed.
    ADD COLUMN on_hold           BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN hold_reason       TEXT        NOT NULL DEFAULT '',
    -- When the team started owing a reply (the member's first unanswered
    -- message); NULL while it's the member's turn.
    ADD COLUMN waiting_since     TIMESTAMPTZ,
    ADD COLUMN staff_reminded_at TIMESTAMPTZ,
    ADD COLUMN reminders_sent    INT         NOT NULL DEFAULT 0;

UPDATE tickets SET waiting_since = last_activity_at WHERE status = 'open' AND waiting_on_staff;
CREATE INDEX tickets_waiting_idx ON tickets (waiting_since) WHERE status = 'open' AND waiting_on_staff AND NOT on_hold;

-- +goose Down
DROP INDEX tickets_waiting_idx;
ALTER TABLE tickets
    DROP COLUMN reminders_sent, DROP COLUMN staff_reminded_at, DROP COLUMN waiting_since,
    DROP COLUMN hold_reason, DROP COLUMN on_hold;
ALTER TABLE ticket_types
    DROP COLUMN reminder_ping, DROP COLUMN reminder_where, DROP COLUMN reminder_repeat,
    DROP COLUMN reminder_minutes, DROP COLUMN reply_target_minutes,
    DROP COLUMN claim_lock_exempt_role_ids, DROP COLUMN claim_lock;
