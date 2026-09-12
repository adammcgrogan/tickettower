package store

import (
	"context"

	"github.com/disgoorg/snowflake/v2"
)

// AccessLevel is what a dashboard role lets its members do.
type AccessLevel string

const (
	// LevelViewer can read everything: Home, Tickets, transcripts, Analytics.
	LevelViewer AccessLevel = "viewer"
	// LevelSupport can also act on tickets: reply, close, reopen, move,
	// hold, and block members.
	LevelSupport AccessLevel = "support"
	// LevelAdmin can change the setup too: ticket types, buttons, saved
	// replies and settings, except who has dashboard access.
	LevelAdmin AccessLevel = "admin"
	// LevelOwner is the server owner, administrators and Manage Server: it
	// isn't granted by a role, and it alone changes dashboard access.
	LevelOwner AccessLevel = "owner"
)

// AccessLevels lists the levels a role can be given, lowest first.
var AccessLevels = []AccessLevel{LevelViewer, LevelSupport, LevelAdmin}

// rank orders levels so they can be compared; unknown levels rank lowest.
func (l AccessLevel) rank() int {
	switch l {
	case LevelViewer:
		return 1
	case LevelSupport:
		return 2
	case LevelAdmin:
		return 3
	case LevelOwner:
		return 4
	}
	return 0
}

// AtLeast reports whether l grants everything min does.
func (l AccessLevel) AtLeast(min AccessLevel) bool { return l.rank() >= min.rank() }

// Valid reports whether l is a level a role can be given.
func (l AccessLevel) Valid() bool { return l == LevelViewer || l == LevelSupport || l == LevelAdmin }

// DashboardRole lets members with a role use the dashboard at a level.
type DashboardRole struct {
	RoleID snowflake.ID `json:"role_id"`
	Level  AccessLevel  `json:"level"`
}

// HighestLevel returns the best level among roles granted to a member with
// memberRoles, or "" if none apply.
func HighestLevel(roles []DashboardRole, memberRoles []snowflake.ID) AccessLevel {
	var best AccessLevel
	for _, r := range roles {
		for _, id := range memberRoles {
			if id == r.RoleID && r.Level.rank() > best.rank() {
				best = r.Level
			}
		}
	}
	return best
}

type GuildSettings struct {
	TranscriptRetentionDays *int `json:"transcript_retention_days"`
	// DashboardRoles let members without Manage Server use the dashboard,
	// each at a level.
	DashboardRoles []DashboardRole `json:"dashboard_roles"`
	// LogChannelID is where ticket events are posted, if set.
	LogChannelID *snowflake.ID `json:"log_channel_id"`
}

func rolesOrEmpty(r []DashboardRole) []DashboardRole {
	if r == nil {
		return []DashboardRole{}
	}
	return r
}

func (s *Store) GetGuildSettings(ctx context.Context, guildID snowflake.ID) (GuildSettings, error) {
	var (
		st         GuildSettings
		logChannel *int64
	)
	err := s.pool.QueryRow(ctx, `
		SELECT transcript_retention_days, dashboard_roles, log_channel_id FROM guild_settings WHERE guild_id = $1`,
		int64(guildID)).Scan(&st.TranscriptRetentionDays, &st.DashboardRoles, &logChannel)
	if notFound(err) == ErrNotFound {
		return GuildSettings{DashboardRoles: []DashboardRole{}}, nil
	}
	st.DashboardRoles = rolesOrEmpty(st.DashboardRoles)
	st.LogChannelID = idFromNullable(logChannel)
	return st, err
}

func (s *Store) UpdateGuildSettings(ctx context.Context, guildID snowflake.ID, st GuildSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_settings (guild_id, transcript_retention_days, dashboard_roles, log_channel_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (guild_id) DO UPDATE
		SET transcript_retention_days = EXCLUDED.transcript_retention_days,
		    dashboard_roles = EXCLUDED.dashboard_roles,
		    log_channel_id = EXCLUDED.log_channel_id,
		    updated_at = now()`,
		int64(guildID), st.TranscriptRetentionDays, rolesOrEmpty(st.DashboardRoles), nullableID(st.LogChannelID))
	return err
}

// DashboardRoles returns the dashboard roles of those guilds in ids that have
// any configured.
func (s *Store) DashboardRoles(ctx context.Context, ids []snowflake.ID) (map[snowflake.ID][]DashboardRole, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT guild_id, dashboard_roles FROM guild_settings
		WHERE guild_id = ANY($1) AND jsonb_array_length(dashboard_roles) > 0`, toInt64s(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[snowflake.ID][]DashboardRole)
	for rows.Next() {
		var (
			id    int64
			roles []DashboardRole
		)
		if err := rows.Scan(&id, &roles); err != nil {
			return nil, err
		}
		out[snowflake.ID(id)] = roles
	}
	return out, rows.Err()
}
