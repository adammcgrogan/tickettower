package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// autoCloseLead is how long before an auto-close the member is warned: a
// quarter of the inactivity window, at most a day. The dashboard describes
// the same rule (autoCloseHint in TicketTypeForm.svelte).
const autoCloseLead = `LEAST(make_interval(hours => tt.auto_close_hours) / 4, interval '24 hours')`

// autoCloseEligible matches open tickets whose type auto-closes and where the
// team is waiting on the member, not the other way round.
const autoCloseEligible = `t.status = 'open' AND NOT t.waiting_on_staff AND NOT t.on_hold AND tt.auto_close_hours IS NOT NULL
	AND ` + inActiveGuild

// inActiveGuild keeps the loops off tickets in servers the bot has left,
// where it can't post or delete anything.
const inActiveGuild = `t.guild_id IN (SELECT id FROM guilds WHERE left_at IS NULL)`

// autoCloseWarnDue matches tickets entering the warning period ($1 is now).
const autoCloseWarnDue = `t.auto_close_warned_at IS NULL
	AND t.last_activity_at + make_interval(hours => tt.auto_close_hours) - ` + autoCloseLead + ` <= $1`

// autoCloseDue matches tickets whose member was warned at least the full
// lead time ago and that have been inactive for the whole window.
const autoCloseDue = `t.auto_close_warned_at IS NOT NULL
	AND t.auto_close_warned_at + ` + autoCloseLead + ` <= $1
	AND t.last_activity_at + make_interval(hours => tt.auto_close_hours) <= $1`

// InactiveTicket is an open ticket that is due an auto-close warning or
// closing.
type InactiveTicket struct {
	ID        int64
	GuildID   snowflake.ID
	ChannelID snowflake.ID
	OpenerID  snowflake.ID
	Hours     int       // the type's auto-close window
	CloseAt   time.Time // when it closes if nobody replies
}

// TicketsToWarn returns tickets that should get a "still need help?" warning.
func (s *Store) TicketsToWarn(ctx context.Context, now time.Time) ([]InactiveTicket, error) {
	return s.inactiveTickets(ctx, autoCloseWarnDue, now)
}

// TicketsToAutoClose returns warned tickets that are now due to close.
func (s *Store) TicketsToAutoClose(ctx context.Context, now time.Time) ([]InactiveTicket, error) {
	return s.inactiveTickets(ctx, autoCloseDue, now)
}

func (s *Store) inactiveTickets(ctx context.Context, due string, now time.Time) ([]InactiveTicket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.guild_id, t.channel_id, t.opener_id, tt.auto_close_hours,
		       GREATEST(t.last_activity_at + make_interval(hours => tt.auto_close_hours),
		                COALESCE(t.auto_close_warned_at, $1) + `+autoCloseLead+`)
		FROM tickets t JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE `+autoCloseEligible+` AND `+due+`
		ORDER BY t.id LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InactiveTicket
	for rows.Next() {
		var (
			it                       InactiveTicket
			guildID, channel, opener int64
		)
		if err := rows.Scan(&it.ID, &guildID, &channel, &opener, &it.Hours, &it.CloseAt); err != nil {
			return nil, err
		}
		it.GuildID, it.ChannelID, it.OpenerID = snowflake.ID(guildID), snowflake.ID(channel), snowflake.ID(opener)
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkAutoCloseWarned records that a ticket's member was warned. It reports
// false if the ticket was already warned or closed.
func (s *Store) MarkAutoCloseWarned(ctx context.Context, ticketID int64, at time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET auto_close_warned_at = $2
		WHERE id = $1 AND status = 'open' AND auto_close_warned_at IS NULL`, ticketID, at)
	return tag.RowsAffected() == 1, err
}

// ClearAutoCloseWarning undoes MarkAutoCloseWarned, e.g. when the warning
// couldn't be posted, so the ticket is never closed without one.
func (s *Store) ClearAutoCloseWarning(ctx context.Context, ticketID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE tickets SET auto_close_warned_at = NULL WHERE id = $1`, ticketID)
	return err
}

// RecordActivity notes a message (or "keep open" click) in a ticket. It
// restarts the auto-close clock and cancels any pending warning. byOpener
// marks the ticket as waiting on staff, which pauses auto-close entirely,
// starts the wait clock if it isn't running, takes the ticket off hold (the
// member has news) and withdraws any close request (the member has more to
// say). A staff message ends the wait and clears any reminder.
func (s *Store) RecordActivity(ctx context.Context, ticketID int64, at time.Time, byOpener bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET last_activity_at = GREATEST(last_activity_at, $2), waiting_on_staff = $3, auto_close_warned_at = NULL,
		    waiting_since = CASE WHEN $3 THEN COALESCE(waiting_since, $2) END,
		    staff_reminded_at = CASE WHEN $3 THEN staff_reminded_at END,
		    reminders_sent = CASE WHEN $3 THEN reminders_sent ELSE 0 END,
		    on_hold = on_hold AND NOT $3,
		    hold_reason = CASE WHEN $3 THEN '' ELSE hold_reason END,
		    close_request_by = CASE WHEN $3 THEN NULL ELSE close_request_by END,
		    close_request_by_name = CASE WHEN $3 THEN '' ELSE close_request_by_name END,
		    close_request_reason = CASE WHEN $3 THEN '' ELSE close_request_reason END,
		    close_request_closes_at = CASE WHEN $3 THEN NULL ELSE close_request_closes_at END
		WHERE id = $1 AND status = 'open'`, ticketID, at, byOpener)
	return err
}

// AutoCloseTicket closes a ticket for inactivity, rechecking that it is still
// due. It reports false if someone replied in the meantime.
func (s *Store) AutoCloseTicket(ctx context.Context, ticketID int64, reason string, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets t SET status = 'closed', close_reason = $3, closed_at = $1, auto_closed = true
		FROM ticket_types tt
		WHERE t.id = $2 AND tt.id = t.ticket_type_id AND `+autoCloseEligible+` AND `+autoCloseDue,
		now, ticketID, reason)
	return tag.RowsAffected() == 1, err
}
