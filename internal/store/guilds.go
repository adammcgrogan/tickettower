package store

import (
	"context"
	"errors"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

type Guild struct {
	ID      snowflake.ID
	Name    string
	Icon    *string
	OwnerID snowflake.ID
}

// UpsertGuild records that the bot is in a guild, creating default settings
// the first time it is seen.
func (s *Store) UpsertGuild(ctx context.Context, g Guild) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO guilds (id, name, icon, owner_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, icon = EXCLUDED.icon, owner_id = EXCLUDED.owner_id,
		    left_at = NULL, updated_at = now()`,
		int64(g.ID), g.Name, g.Icon, int64(g.OwnerID))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO guild_settings (guild_id) VALUES ($1) ON CONFLICT DO NOTHING`, int64(g.ID))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkGuildLeft records that the bot was removed from a guild. Data is kept
// so re-adding the bot restores the configuration.
func (s *Store) MarkGuildLeft(ctx context.Context, id snowflake.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE guilds SET left_at = now() WHERE id = $1 AND left_at IS NULL`, int64(id))
	return err
}

// MarkGuildsLeftExcept marks every active guild not in ids as left. Used after
// connecting, to catch removals that happened while the bot was offline.
func (s *Store) MarkGuildsLeftExcept(ctx context.Context, ids []snowflake.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE guilds SET left_at = now() WHERE left_at IS NULL AND NOT (id = ANY($1))`, toInt64s(ids))
	return err
}

// ActiveGuilds returns the subset of ids the bot is currently in.
func (s *Store) ActiveGuilds(ctx context.Context, ids []snowflake.ID) (map[snowflake.ID]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM guilds WHERE left_at IS NULL AND id = ANY($1)`, toInt64s(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	active := make(map[snowflake.ID]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		active[snowflake.ID(id)] = true
	}
	return active, rows.Err()
}

func (s *Store) GetGuild(ctx context.Context, id snowflake.ID) (Guild, error) {
	var g Guild
	var gid, owner int64
	err := s.pool.QueryRow(ctx, `SELECT id, name, icon, owner_id FROM guilds WHERE id = $1`, int64(id)).
		Scan(&gid, &g.Name, &g.Icon, &owner)
	g.ID, g.OwnerID = snowflake.ID(gid), snowflake.ID(owner)
	return g, notFound(err)
}

// GuildTier returns the guild's active plan, defaulting to "free".
func (s *Store) GuildTier(ctx context.Context, guildID snowflake.ID) (string, error) {
	var tier string
	err := s.pool.QueryRow(ctx, `
		SELECT tier FROM entitlements
		WHERE guild_id = $1 AND (expires_at IS NULL OR expires_at > now())`, int64(guildID)).Scan(&tier)
	if errors.Is(err, pgx.ErrNoRows) {
		return "free", nil
	}
	return tier, err
}
