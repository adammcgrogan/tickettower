package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type Embed struct {
	Author      *EmbedAuthor `json:"author,omitempty"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Footer      string       `json:"footer,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
}

// EmbedAuthor names who wrote an embed, such as the staff member behind a
// reply sent from the dashboard.
type EmbedAuthor struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url,omitempty"`
}

// EmbedField holds, for example, a form answer in a ticket's welcome message.
type EmbedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Attachment struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type,omitempty"`
}

// TicketMessage is a message captured in a ticket, for transcripts.
type TicketMessage struct {
	ID           snowflake.ID `json:"id"`
	TicketID     int64        `json:"-"`
	AuthorID     snowflake.ID `json:"author_id"`
	AuthorName   string       `json:"author_name"`
	AuthorAvatar string       `json:"author_avatar"`
	AuthorBot    bool         `json:"author_bot"`
	// SentBy is the staff member a bot-posted reply was sent for (from the
	// dashboard or /reply); nil when the author wrote it themselves.
	SentBy      *snowflake.ID `json:"sent_by"`
	Content     string        `json:"content"`
	Embeds      []Embed       `json:"embeds"`
	Attachments []Attachment  `json:"attachments"`
	CreatedAt   time.Time     `json:"created_at"`
	EditedAt    *time.Time    `json:"edited_at"`
	DeletedAt   *time.Time    `json:"deleted_at"`
}

// InsertTicketMessage stores a message. Duplicates (e.g. after a gateway
// resume, or the bot capturing a reply the dashboard already saved) are
// ignored, except that a SentBy arriving second is kept.
func (s *Store) InsertTicketMessage(ctx context.Context, m TicketMessage) error {
	if m.Embeds == nil {
		m.Embeds = []Embed{}
	}
	if m.Attachments == nil {
		m.Attachments = []Attachment{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ticket_messages (id, ticket_id, author_id, author_name, author_avatar, author_bot,
		                             content, embeds, attachments, created_at, sent_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET sent_by = COALESCE(ticket_messages.sent_by, EXCLUDED.sent_by)`,
		int64(m.ID), m.TicketID, int64(m.AuthorID), m.AuthorName, m.AuthorAvatar, m.AuthorBot,
		m.Content, m.Embeds, m.Attachments, m.CreatedAt, nullableID(m.SentBy))
	return err
}

// UpdateTicketMessage applies an edit. editedAt is nil for updates Discord
// makes itself, such as adding link previews.
func (s *Store) UpdateTicketMessage(ctx context.Context, id snowflake.ID, content string, embeds []Embed, editedAt *time.Time) error {
	if embeds == nil {
		embeds = []Embed{}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE ticket_messages SET content = $2, embeds = $3, edited_at = COALESCE($4, edited_at) WHERE id = $1`,
		int64(id), content, embeds, editedAt)
	return err
}

// MarkTicketMessageDeleted keeps deleted messages in the transcript, flagged.
func (s *Store) MarkTicketMessageDeleted(ctx context.Context, id snowflake.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE ticket_messages SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, int64(id))
	return err
}

func (s *Store) ListTicketMessages(ctx context.Context, ticketID int64) ([]TicketMessage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, author_id, author_name, author_avatar, author_bot, content, embeds, attachments,
		       created_at, edited_at, deleted_at, sent_by
		FROM ticket_messages WHERE ticket_id = $1 ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TicketMessage{}
	for rows.Next() {
		var m TicketMessage
		var id, author int64
		var sentBy *int64
		if err := rows.Scan(&id, &author, &m.AuthorName, &m.AuthorAvatar, &m.AuthorBot, &m.Content,
			&m.Embeds, &m.Attachments, &m.CreatedAt, &m.EditedAt, &m.DeletedAt, &sentBy); err != nil {
			return nil, err
		}
		m.ID, m.AuthorID, m.TicketID = snowflake.ID(id), snowflake.ID(author), ticketID
		m.SentBy = idFromNullable(sentBy)
		out = append(out, m)
	}
	return out, rows.Err()
}

// TicketRef is the minimal state the bot tracks for each open ticket.
type TicketRef struct {
	ID               int64
	GuildID          snowflake.ID
	ChannelID        snowflake.ID
	OpenerID         snowflake.ID
	HasFirstResponse bool
}

func (s *Store) OpenTicketRefs(ctx context.Context) ([]TicketRef, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, guild_id, channel_id, opener_id, first_response_at IS NOT NULL FROM tickets WHERE status = 'open'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TicketRef
	for rows.Next() {
		var r TicketRef
		var guild, channel, opener int64
		if err := rows.Scan(&r.ID, &guild, &channel, &opener, &r.HasFirstResponse); err != nil {
			return nil, err
		}
		r.GuildID, r.ChannelID, r.OpenerID = snowflake.ID(guild), snowflake.ID(channel), snowflake.ID(opener)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetFirstResponse records when staff first replied, if not already set.
func (s *Store) SetFirstResponse(ctx context.Context, ticketID int64, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets SET first_response_at = $2 WHERE id = $1 AND first_response_at IS NULL`, ticketID, at)
	return err
}

// PurgeExpiredTranscripts deletes stored messages for tickets closed longer
// ago than their guild's retention period.
func (s *Store) PurgeExpiredTranscripts(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM ticket_messages m
		USING tickets t, guild_settings gs
		WHERE m.ticket_id = t.id
		  AND gs.guild_id = t.guild_id
		  AND gs.transcript_retention_days IS NOT NULL
		  AND t.closed_at < now() - make_interval(days => gs.transcript_retention_days)`)
	return tag.RowsAffected(), err
}
