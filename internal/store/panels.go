package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

type PanelStyle string

const (
	PanelButtons  PanelStyle = "buttons"
	PanelDropdown PanelStyle = "dropdown"
)

// Panel is the message members interact with to open tickets.
type Panel struct {
	ID          int64        `json:"id"`
	GuildID     snowflake.ID `json:"guild_id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Color       int          `json:"color"`
	Style       PanelStyle   `json:"style"`
	// ImageURL and ThumbnailURL are optional https images on the embed.
	// Placeholder is the dropdown's prompt; "" uses the default.
	ImageURL      string        `json:"image_url"`
	ThumbnailURL  string        `json:"thumbnail_url"`
	Placeholder   string        `json:"placeholder"`
	ChannelID     *snowflake.ID `json:"channel_id"`
	MessageID     *snowflake.ID `json:"message_id"`
	TicketTypeIDs []int64       `json:"ticket_type_ids"`
	CreatedAt     time.Time     `json:"created_at"`
}

const panelSelect = `
	SELECT p.id, p.guild_id, p.title, p.description, p.color, p.style, p.image_url, p.thumbnail_url, p.placeholder,
	       p.channel_id, p.message_id, p.created_at,
	       COALESCE((SELECT array_agg(l.ticket_type_id ORDER BY l.position)
	                 FROM panel_ticket_types l WHERE l.panel_id = p.id), '{}')
	FROM panels p`

func scanPanel(row pgx.Row) (Panel, error) {
	var (
		p                    Panel
		guildID              int64
		style                string
		channelID, messageID *int64
	)
	err := row.Scan(&p.ID, &guildID, &p.Title, &p.Description, &p.Color, &style, &p.ImageURL, &p.ThumbnailURL,
		&p.Placeholder, &channelID, &messageID, &p.CreatedAt, &p.TicketTypeIDs)
	if err != nil {
		return p, notFound(err)
	}
	p.GuildID = snowflake.ID(guildID)
	p.Style = PanelStyle(style)
	p.ChannelID = idFromNullable(channelID)
	p.MessageID = idFromNullable(messageID)
	return p, nil
}

func (s *Store) queryPanels(ctx context.Context, sql string, args ...any) ([]Panel, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Panel{}
	for rows.Next() {
		p, err := scanPanel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListPanels(ctx context.Context, guildID snowflake.ID) ([]Panel, error) {
	return s.queryPanels(ctx, panelSelect+` WHERE p.guild_id = $1 ORDER BY p.id`, int64(guildID))
}

// PublishedPanelsWithType returns panels posted in Discord that include the
// ticket type, so they can be refreshed when it changes.
func (s *Store) PublishedPanelsWithType(ctx context.Context, guildID snowflake.ID, typeID int64) ([]Panel, error) {
	return s.queryPanels(ctx, panelSelect+`
		WHERE p.guild_id = $1 AND p.message_id IS NOT NULL
		  AND EXISTS (SELECT 1 FROM panel_ticket_types l WHERE l.panel_id = p.id AND l.ticket_type_id = $2)
		ORDER BY p.id`, int64(guildID), typeID)
}

func (s *Store) GetPanel(ctx context.Context, guildID snowflake.ID, id int64) (Panel, error) {
	return scanPanel(s.pool.QueryRow(ctx, panelSelect+` WHERE p.guild_id = $1 AND p.id = $2`, int64(guildID), id))
}

func (s *Store) CountPanels(ctx context.Context, guildID snowflake.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM panels WHERE guild_id = $1`, int64(guildID)).Scan(&n)
	return n, err
}

// CreatePanel inserts p with its ticket types and sets its ID and CreatedAt.
func (s *Store) CreatePanel(ctx context.Context, p *Panel) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO panels (guild_id, title, description, color, style, image_url, thumbnail_url, placeholder)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		int64(p.GuildID), p.Title, p.Description, p.Color, string(p.Style), p.ImageURL, p.ThumbnailURL, p.Placeholder,
	).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return err
	}
	if err := setPanelTypes(ctx, tx, p.GuildID, p.ID, p.TicketTypeIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdatePanel saves the panel's content and ticket types. The published
// message location is changed separately via SetPanelMessage.
func (s *Store) UpdatePanel(ctx context.Context, p Panel) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE panels SET title = $3, description = $4, color = $5, style = $6, image_url = $7, thumbnail_url = $8,
		                  placeholder = $9, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		int64(p.GuildID), p.ID, p.Title, p.Description, p.Color, string(p.Style), p.ImageURL, p.ThumbnailURL,
		p.Placeholder)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := setPanelTypes(ctx, tx, p.GuildID, p.ID, p.TicketTypeIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// setPanelTypes replaces a panel's ticket types, preserving order and
// ignoring IDs from other guilds.
func setPanelTypes(ctx context.Context, tx pgx.Tx, guildID snowflake.ID, panelID int64, typeIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM panel_ticket_types WHERE panel_id = $1`, panelID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO panel_ticket_types (panel_id, ticket_type_id, position)
		SELECT $1, t.id, t.ord
		FROM unnest($2::bigint[]) WITH ORDINALITY AS t(id, ord)
		WHERE EXISTS (SELECT 1 FROM ticket_types tt WHERE tt.id = t.id AND tt.guild_id = $3)`,
		panelID, typeIDs, int64(guildID))
	return err
}

// SetPanelMessage records where the panel is posted; nil clears it.
func (s *Store) SetPanelMessage(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID *snowflake.ID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE panels SET channel_id = $3, message_id = $4, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		int64(guildID), id, nullableID(channelID), nullableID(messageID))
	return err
}

func (s *Store) DeletePanel(ctx context.Context, guildID snowflake.ID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM panels WHERE guild_id = $1 AND id = $2`, int64(guildID), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
