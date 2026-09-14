package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

// MaxBlockReason is the longest reason staff can give for a block. It fits
// a slash command option and is shown to the member.
const MaxBlockReason = 200

// Block stops a member opening any ticket in a server.
type Block struct {
	GuildID       snowflake.ID `json:"guild_id"`
	UserID        snowflake.ID `json:"user_id"`
	UserName      string       `json:"user_name"`
	Reason        string       `json:"reason"`
	BlockedBy     snowflake.ID `json:"blocked_by"`
	BlockedByName string       `json:"blocked_by_name"`
	CreatedAt     time.Time    `json:"created_at"`
	ExpiresAt     *time.Time   `json:"expires_at"` // nil means the block never expires
}

const blockSelect = `SELECT guild_id, user_id, user_name, reason, blocked_by, blocked_by_name, created_at, expires_at FROM ticket_blocks`

func scanBlock(row pgx.Row) (Block, error) {
	var (
		b                     Block
		guildID, userID, byID int64
	)
	if err := row.Scan(&guildID, &userID, &b.UserName, &b.Reason, &byID, &b.BlockedByName, &b.CreatedAt, &b.ExpiresAt); err != nil {
		return b, notFound(err)
	}
	b.GuildID = snowflake.ID(guildID)
	b.UserID = snowflake.ID(userID)
	b.BlockedBy = snowflake.ID(byID)
	return b, nil
}

// ListBlocks returns a guild's active blocked members, newest first.
func (s *Store) ListBlocks(ctx context.Context, guildID snowflake.ID) ([]Block, error) {
	rows, err := s.pool.Query(ctx, blockSelect+`
		WHERE guild_id = $1 AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC, user_id`, int64(guildID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Block{}
	for rows.Next() {
		b, err := scanBlock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBlock returns a member's active block, or ErrNotFound if they aren't
// blocked (including if their block has expired).
func (s *Store) GetBlock(ctx context.Context, guildID, userID snowflake.ID) (Block, error) {
	return scanBlock(s.pool.QueryRow(ctx,
		blockSelect+` WHERE guild_id = $1 AND user_id = $2 AND (expires_at IS NULL OR expires_at > now())`,
		int64(guildID), int64(userID)))
}

// BlockMember blocks a member, or updates the reason, expiry and who blocked
// them if they already were. It sets b.CreatedAt.
func (s *Store) BlockMember(ctx context.Context, b *Block) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO ticket_blocks (guild_id, user_id, user_name, reason, blocked_by, blocked_by_name, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (guild_id, user_id) DO UPDATE
		SET user_name = EXCLUDED.user_name, reason = EXCLUDED.reason,
		    blocked_by = EXCLUDED.blocked_by, blocked_by_name = EXCLUDED.blocked_by_name,
		    expires_at = EXCLUDED.expires_at, created_at = now()
		RETURNING created_at`,
		int64(b.GuildID), int64(b.UserID), b.UserName, b.Reason, int64(b.BlockedBy), b.BlockedByName, b.ExpiresAt).Scan(&b.CreatedAt)
}

// DeleteExpiredBlocks removes blocks whose expiry has passed, so they stop
// showing up in the Blocked members list.
func (s *Store) DeleteExpiredBlocks(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ticket_blocks WHERE expires_at IS NOT NULL AND expires_at <= $1`, now)
	return err
}

// UnblockMember lifts a block. It returns ErrNotFound if there wasn't one.
func (s *Store) UnblockMember(ctx context.Context, guildID, userID snowflake.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ticket_blocks WHERE guild_id = $1 AND user_id = $2`, int64(guildID), int64(userID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LastClosedAt returns when a member's most recent ticket of a type closed,
// or nil if none has.
func (s *Store) LastClosedAt(ctx context.Context, guildID snowflake.ID, typeID int64, userID snowflake.ID) (*time.Time, error) {
	var at *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT max(closed_at) FROM tickets
		WHERE guild_id = $1 AND opener_id = $2 AND ticket_type_id = $3 AND status = 'closed'`,
		int64(guildID), int64(userID), typeID).Scan(&at)
	return at, err
}
