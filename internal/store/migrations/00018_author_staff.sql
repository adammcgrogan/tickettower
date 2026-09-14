-- +goose Up
-- Whether a message came from the team: someone with one of the ticket
-- type's support roles or who manages the server, or a reply the bot posted
-- for a staff member. Everyone else in a ticket (the opener, people added
-- with /ticket add) is the member's side. Existing rows keep the old rule,
-- where anyone but the opener counted as the team.
ALTER TABLE ticket_messages ADD COLUMN author_staff BOOLEAN NOT NULL DEFAULT false;
UPDATE ticket_messages m SET author_staff = true
FROM tickets t
WHERE m.ticket_id = t.id AND ((NOT m.author_bot AND m.author_id <> t.opener_id) OR m.sent_by IS NOT NULL);

-- +goose Down
ALTER TABLE ticket_messages DROP COLUMN author_staff;
