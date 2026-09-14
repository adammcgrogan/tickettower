package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// MaxNoteLength keeps a staff note to a few lines.
const MaxNoteLength = 1000

// TicketNote is a private note staff left on a ticket. Only the team sees
// notes, never the member who opened the ticket.
type TicketNote struct {
	ID         int64        `json:"id"`
	TicketID   int64        `json:"-"`
	AuthorID   snowflake.ID `json:"author_id"`
	AuthorName string       `json:"author_name"`
	Content    string       `json:"content"`
	CreatedAt  time.Time    `json:"created_at"`
}

// AddTicketNote saves a note on one of a guild's tickets and sets its ID and
// CreatedAt. It returns ErrNotFound if the guild has no such ticket.
func (s *Store) AddTicketNote(ctx context.Context, guildID snowflake.ID, n *TicketNote) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ticket_notes (ticket_id, author_id, author_name, content)
		SELECT id, $3, $4, $5 FROM tickets WHERE guild_id = $1 AND id = $2
		RETURNING id, created_at`,
		int64(guildID), n.TicketID, int64(n.AuthorID), n.AuthorName, n.Content).Scan(&n.ID, &n.CreatedAt)
	return notFound(err)
}

// ListTicketNotes returns the notes on one of a guild's tickets, oldest
// first.
func (s *Store) ListTicketNotes(ctx context.Context, guildID snowflake.ID, ticketID int64) ([]TicketNote, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.id, n.author_id, n.author_name, n.content, n.created_at
		FROM ticket_notes n JOIN tickets t ON t.id = n.ticket_id
		WHERE t.guild_id = $1 AND n.ticket_id = $2
		ORDER BY n.created_at, n.id`, int64(guildID), ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TicketNote{}
	for rows.Next() {
		n := TicketNote{TicketID: ticketID}
		var author int64
		if err := rows.Scan(&n.ID, &author, &n.AuthorName, &n.Content, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.AuthorID = snowflake.ID(author)
		out = append(out, n)
	}
	return out, rows.Err()
}
