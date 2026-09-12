package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// WaitingTicket is an open ticket the team has owed a reply for longer than
// its type's reminder time.
type WaitingTicket struct {
	ID             int64
	GuildID        snowflake.ID
	ChannelID      snowflake.ID
	Number         int
	TypeName       string
	OpenerID       snowflake.ID
	ClaimedBy      *snowflake.ID
	SupportRoleIDs []snowflake.ID
	WaitingSince   time.Time
	Minutes        int // the type's reminder time
	Where          ReminderWhere
	Ping           ReminderPing
	RemindersSent  int
}

// TicketsToRemind returns tickets due a staff reminder: waiting on the team,
// not on hold, past the type's reminder time since they started waiting, and
// either never reminded or (if the type repeats) reminded a full interval
// ago.
func (s *Store) TicketsToRemind(ctx context.Context, now time.Time) ([]WaitingTicket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.guild_id, t.channel_id, t.number, t.type_name, t.opener_id, t.claimed_by, tt.support_role_ids,
		       t.waiting_since, tt.reminder_minutes, tt.reminder_where, tt.reminder_ping, t.reminders_sent
		FROM tickets t JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE t.status = 'open' AND t.waiting_on_staff AND NOT t.on_hold AND t.waiting_since IS NOT NULL
		  AND tt.reminder_minutes IS NOT NULL
		  AND t.waiting_since + make_interval(mins => tt.reminder_minutes) <= $1
		  AND (t.staff_reminded_at IS NULL
		       OR (tt.reminder_repeat AND t.staff_reminded_at + make_interval(mins => tt.reminder_minutes) <= $1))
		ORDER BY t.waiting_since LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WaitingTicket
	for rows.Next() {
		var (
			w                        WaitingTicket
			guildID, channel, opener int64
			claimer                  *int64
			roles                    []int64
			where, ping              string
		)
		if err := rows.Scan(&w.ID, &guildID, &channel, &w.Number, &w.TypeName, &opener, &claimer, &roles,
			&w.WaitingSince, &w.Minutes, &where, &ping, &w.RemindersSent); err != nil {
			return nil, err
		}
		w.GuildID, w.ChannelID, w.OpenerID = snowflake.ID(guildID), snowflake.ID(channel), snowflake.ID(opener)
		w.ClaimedBy = idFromNullable(claimer)
		w.SupportRoleIDs = fromInt64s(roles)
		w.Where, w.Ping = ReminderWhere(where), ReminderPing(ping)
		out = append(out, w)
	}
	return out, rows.Err()
}

// MarkReminded records a staff reminder. It reports false if the ticket was
// answered, put on hold or closed in the meantime, or another process got
// there first (the reminder time since the last one hasn't passed).
func (s *Store) MarkReminded(ctx context.Context, ticketID int64, minutes int, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET staff_reminded_at = $2, reminders_sent = reminders_sent + 1
		WHERE id = $1 AND status = 'open' AND waiting_on_staff AND NOT on_hold
		  AND (staff_reminded_at IS NULL OR staff_reminded_at + make_interval(mins => $3) <= $2)`,
		ticketID, now, minutes)
	return tag.RowsAffected() == 1, err
}
