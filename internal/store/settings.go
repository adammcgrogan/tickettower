package store

import (
	"context"

	"github.com/disgoorg/snowflake/v2"
)

type GuildSettings struct {
	TranscriptRetentionDays *int `json:"transcript_retention_days"`
	// DashboardRoleIDs let members without Manage Server use the dashboard.
	DashboardRoleIDs []snowflake.ID `json:"dashboard_role_ids"`
}

func (s *Store) GetGuildSettings(ctx context.Context, guildID snowflake.ID) (GuildSettings, error) {
	var (
		st    GuildSettings
		roles []int64
	)
	err := s.pool.QueryRow(ctx, `
		SELECT transcript_retention_days, dashboard_role_ids FROM guild_settings WHERE guild_id = $1`,
		int64(guildID)).Scan(&st.TranscriptRetentionDays, &roles)
	if notFound(err) == ErrNotFound {
		return GuildSettings{DashboardRoleIDs: []snowflake.ID{}}, nil
	}
	st.DashboardRoleIDs = fromInt64s(roles)
	return st, err
}

func (s *Store) UpdateGuildSettings(ctx context.Context, guildID snowflake.ID, st GuildSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_settings (guild_id, transcript_retention_days, dashboard_role_ids) VALUES ($1, $2, $3)
		ON CONFLICT (guild_id) DO UPDATE
		SET transcript_retention_days = EXCLUDED.transcript_retention_days,
		    dashboard_role_ids = EXCLUDED.dashboard_role_ids,
		    updated_at = now()`,
		int64(guildID), st.TranscriptRetentionDays, toInt64s(st.DashboardRoleIDs))
	return err
}

// DashboardRoles returns the dashboard roles of those guilds in ids that have
// any configured.
func (s *Store) DashboardRoles(ctx context.Context, ids []snowflake.ID) (map[snowflake.ID][]snowflake.ID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT guild_id, dashboard_role_ids FROM guild_settings
		WHERE guild_id = ANY($1) AND cardinality(dashboard_role_ids) > 0`, toInt64s(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[snowflake.ID][]snowflake.ID)
	for rows.Next() {
		var (
			id    int64
			roles []int64
		)
		if err := rows.Scan(&id, &roles); err != nil {
			return nil, err
		}
		out[snowflake.ID(id)] = fromInt64s(roles)
	}
	return out, rows.Err()
}
