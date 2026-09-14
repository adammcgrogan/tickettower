package store

import (
	"context"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

type TicketStatus string

const (
	StatusOpen   TicketStatus = "open"
	StatusClosed TicketStatus = "closed"
)

type Ticket struct {
	ID              int64         `json:"id"`
	GuildID         snowflake.ID  `json:"guild_id"`
	Number          int           `json:"number"`
	TicketTypeID    *int64        `json:"ticket_type_id"`
	TypeName        string        `json:"type_name"`
	Mode            TicketMode    `json:"mode"`
	ChannelID       snowflake.ID  `json:"channel_id"`
	OpenerID        snowflake.ID  `json:"opener_id"`
	OpenerName      string        `json:"opener_name"`
	ClaimedBy       *snowflake.ID `json:"claimed_by"`
	ClaimedByName   *string       `json:"claimed_by_name"`
	Status          TicketStatus  `json:"status"`
	CloseReason     string        `json:"close_reason"`
	ClosedBy        *snowflake.ID `json:"closed_by"`
	ClosedByName    *string       `json:"closed_by_name"`
	OpenedAt        time.Time     `json:"opened_at"`
	FirstResponseAt *time.Time    `json:"first_response_at"`
	ClosedAt        *time.Time    `json:"closed_at"`
	LastActivityAt  time.Time     `json:"last_activity_at"`
	// WaitingOnStaff is true when the opener sent the last message, which
	// pauses auto-close. WaitingSince is when that started (the member's
	// first unanswered message); nil while it's the member's turn.
	WaitingOnStaff bool       `json:"waiting_on_staff"`
	WaitingSince   *time.Time `json:"waiting_since"`
	// OnHold tickets are waiting on something other than the member or the
	// team. They leave the team's queue, reminders and auto-close until the
	// member writes again or staff resume them.
	OnHold     bool   `json:"on_hold"`
	HoldReason string `json:"hold_reason"`
	// Match is set by ListTickets when a search matched something said in
	// the ticket rather than its number or names.
	Match *MessageMatch `json:"match,omitempty"`
}

const ticketColumns = `id, guild_id, number, ticket_type_id, type_name, mode, channel_id, opener_id, opener_name,
	claimed_by, claimed_by_name, status, close_reason, closed_by, closed_by_name, opened_at, first_response_at, closed_at,
	last_activity_at, waiting_on_staff, waiting_since, on_hold, hold_reason`

func scanTicket(row pgx.Row) (Ticket, error) {
	var (
		t                          Ticket
		guildID, channelID, opener int64
		mode, status               string
		claimedBy, closedBy        *int64
	)
	err := row.Scan(&t.ID, &guildID, &t.Number, &t.TicketTypeID, &t.TypeName, &mode, &channelID, &opener,
		&t.OpenerName, &claimedBy, &t.ClaimedByName, &status, &t.CloseReason, &closedBy, &t.ClosedByName, &t.OpenedAt,
		&t.FirstResponseAt, &t.ClosedAt, &t.LastActivityAt, &t.WaitingOnStaff, &t.WaitingSince, &t.OnHold, &t.HoldReason)
	if err != nil {
		return t, notFound(err)
	}
	t.GuildID = snowflake.ID(guildID)
	t.ChannelID = snowflake.ID(channelID)
	t.OpenerID = snowflake.ID(opener)
	t.Mode = TicketMode(mode)
	t.Status = TicketStatus(status)
	t.ClaimedBy = idFromNullable(claimedBy)
	t.ClosedBy = idFromNullable(closedBy)
	return t, nil
}

// NextTicketNumber allocates the guild's next sequential ticket number.
func (s *Store) NextTicketNumber(ctx context.Context, guildID snowflake.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO guild_settings (guild_id, ticket_counter) VALUES ($1, 1)
		ON CONFLICT (guild_id) DO UPDATE SET ticket_counter = guild_settings.ticket_counter + 1
		RETURNING ticket_counter`, int64(guildID)).Scan(&n)
	return n, err
}

// OpenTicketChannels returns the channels of a user's open tickets of a type.
func (s *Store) OpenTicketChannels(ctx context.Context, guildID snowflake.ID, typeID int64, userID snowflake.ID) ([]snowflake.ID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT channel_id FROM tickets
		WHERE guild_id = $1 AND ticket_type_id = $2 AND opener_id = $3 AND status = 'open'
		ORDER BY opened_at`, int64(guildID), typeID, int64(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []snowflake.ID
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, snowflake.ID(id))
	}
	return out, rows.Err()
}

// CreateTicket inserts t and sets its ID, Status, OpenedAt, LastActivityAt
// and, if it starts waiting on staff, WaitingSince.
func (s *Store) CreateTicket(ctx context.Context, t *Ticket) error {
	var status string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tickets (guild_id, number, ticket_type_id, type_name, mode, channel_id, opener_id, opener_name,
		                     waiting_on_staff, waiting_since)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CASE WHEN $9 THEN now() END)
		RETURNING id, status, opened_at, last_activity_at, waiting_since`,
		int64(t.GuildID), t.Number, t.TicketTypeID, t.TypeName, string(t.Mode), int64(t.ChannelID),
		int64(t.OpenerID), t.OpenerName, t.WaitingOnStaff,
	).Scan(&t.ID, &status, &t.OpenedAt, &t.LastActivityAt, &t.WaitingSince)
	t.Status = TicketStatus(status)
	return err
}

// HoldTicket puts an open ticket on hold. It reports false if the ticket is
// closed or already on hold.
func (s *Store) HoldTicket(ctx context.Context, id int64, reason string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET on_hold = true, hold_reason = $2
		WHERE id = $1 AND status = 'open' AND NOT on_hold`, id, reason)
	return tag.RowsAffected() == 1, err
}

// ResumeTicket takes a ticket off hold. It reports false if it wasn't on
// hold. The wait clock restarts, so a long hold doesn't count against the
// team.
func (s *Store) ResumeTicket(ctx context.Context, id int64, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET on_hold = false, hold_reason = '', staff_reminded_at = NULL, reminders_sent = 0,
		    waiting_since = CASE WHEN waiting_on_staff THEN $2::timestamptz END
		WHERE id = $1 AND status = 'open' AND on_hold`, id, now)
	return tag.RowsAffected() == 1, err
}

func (s *Store) GetTicket(ctx context.Context, id int64) (Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE id = $1`, id))
}

// GetGuildTicket returns one of a guild's tickets.
func (s *Store) GetGuildTicket(ctx context.Context, guildID snowflake.ID, id int64) (Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE guild_id = $1 AND id = $2`,
		int64(guildID), id))
}

func (s *Store) GetTicketByChannel(ctx context.Context, channelID snowflake.ID) (Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE channel_id = $1`, int64(channelID)))
}

// ClaimTicket claims an open, unclaimed ticket. It reports false if someone
// else got there first.
func (s *Store) ClaimTicket(ctx context.Context, id int64, userID snowflake.ID, userName string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET claimed_by = $2, claimed_by_name = $3
		WHERE id = $1 AND status = 'open' AND claimed_by IS NULL`, id, int64(userID), userName)
	return tag.RowsAffected() == 1, err
}

// UnclaimTicket releases a ticket claimed by userID.
func (s *Store) UnclaimTicket(ctx context.Context, id int64, userID snowflake.ID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET claimed_by = NULL, claimed_by_name = NULL
		WHERE id = $1 AND status = 'open' AND claimed_by = $2`, id, int64(userID))
	return tag.RowsAffected() == 1, err
}

// CloseTicket closes an open ticket. It reports false if it was already
// closed, which makes concurrent closes safe.
func (s *Store) CloseTicket(ctx context.Context, id int64, closedBy snowflake.ID, closedByName, reason string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET status = 'closed', closed_by = $2, closed_by_name = $3, close_reason = $4, closed_at = now()
		WHERE id = $1 AND status = 'open'`, id, int64(closedBy), closedByName, reason)
	return tag.RowsAffected() == 1, err
}

// CloseTicketByChannel closes the open ticket in a channel, if any. Used when
// a ticket channel is deleted outside the bot.
func (s *Store) CloseTicketByChannel(ctx context.Context, channelID snowflake.ID, reason string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET status = 'closed', close_reason = $2, closed_at = now(), channel_cleaned_at = now()
		WHERE channel_id = $1 AND status = 'open'`, int64(channelID), reason)
	return tag.RowsAffected() == 1, err
}

// CloseTicketsOfLeftGuilds closes every open ticket in guilds the bot has
// left, since it can no longer post in, warn about or delete their channels.
// Their channels count as dealt with. It returns how many were closed.
func (s *Store) CloseTicketsOfLeftGuilds(ctx context.Context, reason string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets t SET status = 'closed', close_reason = $1, closed_at = now(), channel_cleaned_at = now()
		FROM guilds g
		WHERE g.id = t.guild_id AND g.left_at IS NOT NULL AND t.status = 'open'`, reason)
	return tag.RowsAffected(), err
}

// ReopenTicket reopens a closed thread ticket. It reports false if the
// ticket is open, or is a channel ticket (its channel is gone). The ticket
// starts as waiting on the team, with any auto-close warning forgotten.
func (s *Store) ReopenTicket(ctx context.Context, id int64, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET status = 'open', closed_by = NULL, closed_by_name = NULL, close_reason = '', closed_at = NULL,
		    auto_closed = false, channel_cleaned_at = NULL, auto_close_warned_at = NULL,
		    waiting_on_staff = true, waiting_since = $2, last_activity_at = $2, reopened_at = $2,
		    on_hold = false, hold_reason = '', staff_reminded_at = NULL, reminders_sent = 0
		WHERE id = $1 AND status = 'closed' AND mode = 'thread'`, id, now)
	return tag.RowsAffected() == 1, err
}

// TicketsToCleanUp returns closed tickets whose channel the bot hasn't
// finished with yet (deleted, or archived for threads), closed before the
// given time so a close still in progress isn't picked up.
func (s *Store) TicketsToCleanUp(ctx context.Context, closedBefore time.Time) ([]Ticket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+ticketColumns+` FROM tickets
		WHERE status = 'closed' AND channel_cleaned_at IS NULL AND closed_at <= $1
		  AND guild_id IN (SELECT id FROM guilds WHERE left_at IS NULL)
		ORDER BY closed_at LIMIT 50`, closedBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkChannelCleaned records that a closed ticket's channel is dealt with.
func (s *Store) MarkChannelCleaned(ctx context.Context, ticketID int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE tickets SET channel_cleaned_at = now() WHERE id = $1 AND channel_cleaned_at IS NULL`, ticketID)
	return err
}

// MoveTicket moves an open ticket to another ticket type. It reports false if
// the ticket has closed.
func (s *Store) MoveTicket(ctx context.Context, guildID snowflake.ID, id, typeID int64, typeName string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET ticket_type_id = $3, type_name = $4
		WHERE guild_id = $1 AND id = $2 AND status = 'open'`, int64(guildID), id, typeID, typeName)
	return tag.RowsAffected() == 1, err
}

// TicketQuery picks which of a guild's tickets ListTickets returns.
type TicketQuery struct {
	Status TicketStatus // "" for any
	TypeID *int64
	// Search matches the ticket number, the opener, claimer and type names,
	// and words in the ticket's messages.
	Search string
	// Before continues the list after this ticket, for paging.
	Before *int64
	Limit  int
}

// likeEscaper makes a search term match literally in a LIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ListTickets returns the guild's tickets matching q, newest first. Tickets
// found through their messages carry a Match with a snippet.
func (s *Store) ListTickets(ctx context.Context, guildID snowflake.ID, q TicketQuery) ([]Ticket, error) {
	pattern, words := "", ""
	if q.Search != "" {
		pattern = "%" + likeEscaper.Replace(q.Search) + "%"
		words = searchQuery(q.Search)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+ticketColumns+` FROM tickets
		WHERE guild_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3::bigint IS NULL OR ticket_type_id = $3)
		  AND ($4 = '' OR number::text LIKE $4 OR opener_name ILIKE $4 OR type_name ILIKE $4
		       OR claimed_by_name ILIKE $4
		       OR ($7 <> '' AND EXISTS (SELECT 1 FROM ticket_messages m
		                                WHERE m.ticket_id = tickets.id AND m.deleted_at IS NULL
		                                  AND m.search @@ to_tsquery('simple', $7))))
		  AND ($5::bigint IS NULL
		       OR (opened_at, id) < (SELECT opened_at, id FROM tickets WHERE guild_id = $1 AND id = $5))
		ORDER BY opened_at DESC, id DESC LIMIT $6`,
		int64(guildID), string(q.Status), q.TypeID, pattern, q.Before, q.Limit, words)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if words == "" {
		return out, nil
	}
	ids := make([]int64, len(out))
	for i, t := range out {
		ids[i] = t.ID
	}
	matches, err := s.messageMatches(ctx, ids, words)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if m, ok := matches[out[i].ID]; ok {
			out[i].Match = &m
		}
	}
	return out, nil
}

type TicketStats struct {
	Open       int `json:"open"`
	OpenedWeek int `json:"opened_week"`
}

func (s *Store) TicketStats(ctx context.Context, guildID snowflake.ID) (TicketStats, error) {
	var st TicketStats
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'open'),
		       count(*) FILTER (WHERE opened_at > now() - interval '7 days')
		FROM tickets WHERE guild_id = $1`, int64(guildID)).Scan(&st.Open, &st.OpenedWeek)
	return st, err
}
