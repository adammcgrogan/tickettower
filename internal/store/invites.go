package store

import (
	"context"
)

// InviteSource is how many times the invite link was clicked from one
// source in a window.
type InviteSource struct {
	Source string `json:"source"`
	Clicks int    `json:"clicks"`
}

// CountInviteClick adds one click for source today (UTC).
func (s *Store) CountInviteClick(ctx context.Context, source string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO invite_clicks (day, source, clicks) VALUES ((now() AT TIME ZONE 'utc')::date, $1, 1)
		ON CONFLICT (day, source) DO UPDATE SET clicks = invite_clicks.clicks + 1`, source)
	return err
}

// InviteSources totals invite clicks per source over the last days days,
// most clicked first.
func (s *Store) InviteSources(ctx context.Context, days int) ([]InviteSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source, sum(clicks)::int
		FROM invite_clicks
		WHERE day > (now() AT TIME ZONE 'utc')::date - $1::int
		GROUP BY source
		ORDER BY 2 DESC, 1`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InviteSource{}
	for rows.Next() {
		var src InviteSource
		if err := rows.Scan(&src.Source, &src.Clicks); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// ActiveGuildCount is how many servers the bot is in right now, as posted to
// bot list sites.
func (s *Store) ActiveGuildCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM guilds WHERE left_at IS NULL`).Scan(&n)
	return n, err
}
