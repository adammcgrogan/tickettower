package store

import (
	"context"
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
	// pauses auto-close.
	WaitingOnStaff bool `json:"waiting_on_staff"`
}

const ticketColumns = `id, guild_id, number, ticket_type_id, type_name, mode, channel_id, opener_id, opener_name,
	claimed_by, claimed_by_name, status, close_reason, closed_by, closed_by_name, opened_at, first_response_at, closed_at,
	last_activity_at, waiting_on_staff`

func scanTicket(row pgx.Row) (Ticket, error) {
	var (
		t                          Ticket
		guildID, channelID, opener int64
		mode, status               string
		claimedBy, closedBy        *int64
	)
	err := row.Scan(&t.ID, &guildID, &t.Number, &t.TicketTypeID, &t.TypeName, &mode, &channelID, &opener,
		&t.OpenerName, &claimedBy, &t.ClaimedByName, &status, &t.CloseReason, &closedBy, &t.ClosedByName, &t.OpenedAt,
		&t.FirstResponseAt, &t.ClosedAt, &t.LastActivityAt, &t.WaitingOnStaff)
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

// CreateTicket inserts t and sets its ID, Status, OpenedAt and LastActivityAt.
func (s *Store) CreateTicket(ctx context.Context, t *Ticket) error {
	var status string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tickets (guild_id, number, ticket_type_id, type_name, mode, channel_id, opener_id, opener_name,
		                     waiting_on_staff)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, status, opened_at, last_activity_at`,
		int64(t.GuildID), t.Number, t.TicketTypeID, t.TypeName, string(t.Mode), int64(t.ChannelID),
		int64(t.OpenerID), t.OpenerName, t.WaitingOnStaff,
	).Scan(&t.ID, &status, &t.OpenedAt, &t.LastActivityAt)
	t.Status = TicketStatus(status)
	return err
}

func (s *Store) GetTicket(ctx context.Context, id int64) (Ticket, error) {
	return scanTicket(s.pool.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE id = $1`, id))
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
		UPDATE tickets SET status = 'closed', close_reason = $2, closed_at = now()
		WHERE channel_id = $1 AND status = 'open'`, int64(channelID), reason)
	return tag.RowsAffected() == 1, err
}

// ListTickets returns the guild's most recent tickets, optionally filtered by
// status.
func (s *Store) ListTickets(ctx context.Context, guildID snowflake.ID, status TicketStatus, limit int) ([]Ticket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+ticketColumns+` FROM tickets
		WHERE guild_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY opened_at DESC LIMIT $3`, int64(guildID), string(status), limit)
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
	return out, rows.Err()
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
