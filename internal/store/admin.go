package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// AdminOverview is the headline numbers for the superadmin panel: how many
// servers have the bot, how that's moving, what plan they're on and how much
// they're actually using it.
type AdminOverview struct {
	TotalGuilds  int `json:"total_guilds"`  // ever installed, including ones that left
	ActiveGuilds int `json:"active_guilds"` // bot currently installed
	JoinedLast30 int `json:"joined_last_30"`
	LeftLast30   int `json:"left_last_30"`
	// Among active guilds.
	PremiumGuilds int `json:"premium_guilds"`
	FreeGuilds    int `json:"free_guilds"`

	TicketsOpenedLast30 int `json:"tickets_opened_last_30"`
	TicketsClosedLast30 int `json:"tickets_closed_last_30"`
	// UsingGuilds30 is how many active guilds opened at least one ticket in
	// the last 30 days, i.e. are actually using the bot rather than just
	// having it sit there unconfigured.
	UsingGuilds30 int `json:"using_guilds_last_30"`
}

// AdminOverview summarises install, plan and usage numbers across every
// guild for the superadmin panel.
func (s *Store) AdminOverview(ctx context.Context) (AdminOverview, error) {
	var o AdminOverview
	err := s.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE left_at IS NULL),
		       count(*) FILTER (WHERE left_at IS NULL AND joined_at >= now() - interval '30 days'),
		       count(*) FILTER (WHERE left_at IS NOT NULL AND left_at >= now() - interval '30 days')
		FROM guilds`).Scan(&o.TotalGuilds, &o.ActiveGuilds, &o.JoinedLast30, &o.LeftLast30)
	if err != nil {
		return o, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE e.tier = 'premium'), count(*) FILTER (WHERE e.tier IS NULL OR e.tier <> 'premium')
		FROM guilds g LEFT JOIN entitlements e ON e.guild_id = g.id AND (e.expires_at IS NULL OR e.expires_at > now())
		WHERE g.left_at IS NULL`).Scan(&o.PremiumGuilds, &o.FreeGuilds)
	if err != nil {
		return o, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE opened_at >= now() - interval '30 days'),
		       count(*) FILTER (WHERE closed_at >= now() - interval '30 days'),
		       count(DISTINCT guild_id) FILTER (WHERE opened_at >= now() - interval '30 days')
		FROM tickets`).Scan(&o.TicketsOpenedLast30, &o.TicketsClosedLast30, &o.UsingGuilds30)
	return o, err
}

// AdminGuild is one row of the superadmin guild list.
type AdminGuild struct {
	ID            snowflake.ID `json:"id"`
	Name          string       `json:"name"`
	Icon          *string      `json:"icon"`
	Tier          string       `json:"tier"`
	JoinedAt      time.Time    `json:"joined_at"`
	LeftAt        *time.Time   `json:"left_at"`
	TicketsTotal  int          `json:"tickets_total"`
	TicketsLast30 int          `json:"tickets_last_30"`
	LastTicketAt  *time.Time   `json:"last_ticket_at"`
}

// AdminGuilds lists every guild the bot has ever been in, most recently
// joined first, with its plan and how much it's used the bot.
func (s *Store) AdminGuilds(ctx context.Context) ([]AdminGuild, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, g.icon, COALESCE(e.tier, 'free'), g.joined_at, g.left_at,
		       COALESCE(t.total, 0), COALESCE(t.last_30, 0), t.last_ticket_at
		FROM guilds g
		LEFT JOIN entitlements e ON e.guild_id = g.id AND (e.expires_at IS NULL OR e.expires_at > now())
		LEFT JOIN (
			SELECT guild_id, count(*) AS total,
			       count(*) FILTER (WHERE opened_at >= now() - interval '30 days') AS last_30,
			       max(opened_at) AS last_ticket_at
			FROM tickets GROUP BY guild_id
		) t ON t.guild_id = g.id
		ORDER BY g.joined_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AdminGuild
	for rows.Next() {
		var g AdminGuild
		var id int64
		if err := rows.Scan(&id, &g.Name, &g.Icon, &g.Tier, &g.JoinedAt, &g.LeftAt,
			&g.TicketsTotal, &g.TicketsLast30, &g.LastTicketAt); err != nil {
			return nil, err
		}
		g.ID = snowflake.ID(id)
		out = append(out, g)
	}
	if out == nil {
		out = []AdminGuild{}
	}
	return out, rows.Err()
}

// SetGuildTier sets or clears a guild's plan by hand (there's no billing
// integration yet). "free" removes the entitlements row rather than writing
// one, since that's already the default for a guild with none.
func (s *Store) SetGuildTier(ctx context.Context, guildID snowflake.ID, tier string) error {
	if tier == "free" {
		_, err := s.pool.Exec(ctx, `DELETE FROM entitlements WHERE guild_id = $1`, int64(guildID))
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO entitlements (guild_id, tier, source) VALUES ($1, $2, 'manual')
		ON CONFLICT (guild_id) DO UPDATE SET tier = EXCLUDED.tier, source = 'manual', expires_at = NULL`,
		int64(guildID), tier)
	return err
}
