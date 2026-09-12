package store

import (
	"context"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// MaxSavedReplyName fits a Discord autocomplete choice.
	MaxSavedReplyName = 100
	// MaxSavedReplyContent is Discord's limit on a message's text.
	MaxSavedReplyContent = 2000
)

// ErrDuplicateName means the guild already has a saved reply with that name
// (ignoring case).
var ErrDuplicateName = errors.New("name already taken")

// SavedReply is a message a team writes once and sends into tickets.
type SavedReply struct {
	ID        int64        `json:"id"`
	GuildID   snowflake.ID `json:"guild_id"`
	Name      string       `json:"name"`
	Content   string       `json:"content"`
	UpdatedAt time.Time    `json:"updated_at"`
}

const savedReplySelect = `SELECT id, guild_id, name, content, updated_at FROM saved_replies`

func scanSavedReply(row pgx.Row) (SavedReply, error) {
	var (
		r       SavedReply
		guildID int64
	)
	if err := row.Scan(&r.ID, &guildID, &r.Name, &r.Content, &r.UpdatedAt); err != nil {
		return r, notFound(err)
	}
	r.GuildID = snowflake.ID(guildID)
	return r, nil
}

// duplicate turns a unique violation on the name into ErrDuplicateName.
func duplicate(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	return err
}

// ListSavedReplies returns a guild's saved replies in name order.
func (s *Store) ListSavedReplies(ctx context.Context, guildID snowflake.ID) ([]SavedReply, error) {
	rows, err := s.pool.Query(ctx, savedReplySelect+` WHERE guild_id = $1 ORDER BY lower(name), id`, int64(guildID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SavedReply{}
	for rows.Next() {
		r, err := scanSavedReply(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetSavedReply(ctx context.Context, guildID snowflake.ID, id int64) (SavedReply, error) {
	return scanSavedReply(s.pool.QueryRow(ctx, savedReplySelect+` WHERE guild_id = $1 AND id = $2`, int64(guildID), id))
}

func (s *Store) CountSavedReplies(ctx context.Context, guildID snowflake.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM saved_replies WHERE guild_id = $1`, int64(guildID)).Scan(&n)
	return n, err
}

// CreateSavedReply inserts r and sets its ID and UpdatedAt.
func (s *Store) CreateSavedReply(ctx context.Context, r *SavedReply) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO saved_replies (guild_id, name, content) VALUES ($1, $2, $3)
		RETURNING id, updated_at`,
		int64(r.GuildID), r.Name, r.Content).Scan(&r.ID, &r.UpdatedAt)
	return duplicate(err)
}

// UpdateSavedReply saves r's name and content and sets its UpdatedAt.
func (s *Store) UpdateSavedReply(ctx context.Context, r *SavedReply) error {
	err := s.pool.QueryRow(ctx, `
		UPDATE saved_replies SET name = $3, content = $4, updated_at = now()
		WHERE guild_id = $1 AND id = $2
		RETURNING updated_at`,
		int64(r.GuildID), r.ID, r.Name, r.Content).Scan(&r.UpdatedAt)
	return duplicate(notFound(err))
}

func (s *Store) DeleteSavedReply(ctx context.Context, guildID snowflake.ID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM saved_replies WHERE guild_id = $1 AND id = $2`, int64(guildID), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
