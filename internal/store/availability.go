package store

import (
	"context"
	"errors"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

// AvailableStaff is a member who has opted in to getting tickets assigned to
// them automatically, in guilds where a ticket type has auto-assign on.
type AvailableStaff struct {
	GuildID  snowflake.ID `json:"guild_id"`
	UserID   snowflake.ID `json:"user_id"`
	UserName string       `json:"user_name"`
}

// SetAvailable adds or removes a member from a guild's auto-assign pool.
func (s *Store) SetAvailable(ctx context.Context, guildID, userID snowflake.ID, userName string, available bool) error {
	if !available {
		_, err := s.pool.Exec(ctx,
			`DELETE FROM available_staff WHERE guild_id = $1 AND user_id = $2`, int64(guildID), int64(userID))
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO available_staff (guild_id, user_id, user_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (guild_id, user_id) DO UPDATE SET user_name = EXCLUDED.user_name`,
		int64(guildID), int64(userID), userName)
	return err
}

// IsAvailable reports whether a member has opted in to auto-assignment.
func (s *Store) IsAvailable(ctx context.Context, guildID, userID snowflake.ID) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM available_staff WHERE guild_id = $1 AND user_id = $2`, int64(guildID), int64(userID)).Scan(&n)
	return n > 0, err
}

// ListAvailableStaff returns a guild's auto-assign pool.
func (s *Store) ListAvailableStaff(ctx context.Context, guildID snowflake.ID) ([]AvailableStaff, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id, user_name FROM available_staff WHERE guild_id = $1 ORDER BY user_name`, int64(guildID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AvailableStaff{}
	for rows.Next() {
		var a AvailableStaff
		var userID int64
		if err := rows.Scan(&userID, &a.UserName); err != nil {
			return nil, err
		}
		a.GuildID = guildID
		a.UserID = snowflake.ID(userID)
		out = append(out, a)
	}
	return out, rows.Err()
}

// NextAssignee picks the member of a guild's auto-assign pool who was
// assigned longest ago (or never), and records them as just assigned, so
// tickets are shared out round robin. It reports false if the pool is empty.
func (s *Store) NextAssignee(ctx context.Context, guildID snowflake.ID) (userID snowflake.ID, userName string, ok bool, err error) {
	var uid int64
	err = s.pool.QueryRow(ctx, `
		UPDATE available_staff SET last_assigned_at = now()
		WHERE guild_id = $1 AND user_id = (
			SELECT user_id FROM available_staff
			WHERE guild_id = $1
			ORDER BY last_assigned_at NULLS FIRST, user_id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING user_id, user_name`, int64(guildID)).Scan(&uid, &userName)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return snowflake.ID(uid), userName, true, nil
}
