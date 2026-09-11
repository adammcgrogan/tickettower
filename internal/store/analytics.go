package store

import (
	"context"

	"github.com/disgoorg/snowflake/v2"
)

// SetFeedbackRating records (or changes) the opener's rating for a ticket.
func (s *Store) SetFeedbackRating(ctx context.Context, ticketID int64, rating int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ticket_feedback (ticket_id, rating) VALUES ($1, $2)
		ON CONFLICT (ticket_id) DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()`, ticketID, rating)
	return err
}

// SetFeedbackComment adds a comment to an existing rating. It reports false
// if the ticket hasn't been rated.
func (s *Store) SetFeedbackComment(ctx context.Context, ticketID int64, comment string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ticket_feedback SET comment = $2, updated_at = now() WHERE ticket_id = $1`, ticketID, comment)
	return tag.RowsAffected() == 1, err
}

type DailyCount struct {
	Date   string `json:"date"` // YYYY-MM-DD, UTC
	Opened int    `json:"opened"`
	Closed int    `json:"closed"`
}

type TypeCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type StaffStat struct {
	UserID    snowflake.ID `json:"user_id"`
	Name      string       `json:"name"`
	Claimed   int          `json:"claimed"`
	Closed    int          `json:"closed"`
	AvgRating *float64     `json:"avg_rating"`
}

type Analytics struct {
	Days                   int          `json:"days"`
	Daily                  []DailyCount `json:"daily"`
	Opened                 int          `json:"opened"`
	Closed                 int          `json:"closed"`
	FirstResponseMedianSec *float64     `json:"first_response_median_seconds"`
	ResolutionMedianSec    *float64     `json:"resolution_median_seconds"`
	RatingAvg              *float64     `json:"rating_avg"`
	RatingCount            int          `json:"rating_count"`
	ByType                 []TypeCount  `json:"by_type"`
	Staff                  []StaffStat  `json:"staff"`
}

// since is the start of the reporting window: midnight UTC, days-1 days ago.
const since = `(date_trunc('day', now()) - make_interval(days => $2 - 1))`

// Analytics summarises a guild's tickets over the last `days` days.
func (s *Store) Analytics(ctx context.Context, guildID snowflake.ID, days int) (Analytics, error) {
	a := Analytics{Days: days, Daily: []DailyCount{}, ByType: []TypeCount{}, Staff: []StaffStat{}}
	gid := int64(guildID)

	rows, err := s.pool.Query(ctx, `
		SELECT to_char(d, 'YYYY-MM-DD'),
		       (SELECT count(*) FROM tickets WHERE guild_id = $1 AND opened_at >= d AND opened_at < d + interval '1 day'),
		       (SELECT count(*) FROM tickets WHERE guild_id = $1 AND closed_at >= d AND closed_at < d + interval '1 day')
		FROM generate_series(`+since+`, date_trunc('day', now()), interval '1 day') d
		ORDER BY d`, gid, days)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var d DailyCount
		if err := rows.Scan(&d.Date, &d.Opened, &d.Closed); err != nil {
			rows.Close()
			return a, err
		}
		a.Daily = append(a.Daily, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return a, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE opened_at >= `+since+`),
		       count(*) FILTER (WHERE closed_at >= `+since+`),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM first_response_at - opened_at))
		           FILTER (WHERE opened_at >= `+since+` AND first_response_at IS NOT NULL),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM closed_at - opened_at))
		           FILTER (WHERE closed_at >= `+since+`)
		FROM tickets WHERE guild_id = $1`, gid, days).
		Scan(&a.Opened, &a.Closed, &a.FirstResponseMedianSec, &a.ResolutionMedianSec)
	if err != nil {
		return a, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT avg(f.rating)::float8, count(*)
		FROM ticket_feedback f JOIN tickets t ON t.id = f.ticket_id
		WHERE t.guild_id = $1 AND f.created_at >= `+since, gid, days).Scan(&a.RatingAvg, &a.RatingCount)
	if err != nil {
		return a, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT type_name, count(*) FROM tickets
		WHERE guild_id = $1 AND opened_at >= `+since+`
		GROUP BY type_name ORDER BY count(*) DESC, type_name LIMIT 10`, gid, days)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var tc TypeCount
		if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
			rows.Close()
			return a, err
		}
		a.ByType = append(a.ByType, tc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return a, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT t.claimed_by, max(t.claimed_by_name), count(*),
		       count(*) FILTER (WHERE t.status = 'closed'), avg(f.rating)::float8
		FROM tickets t LEFT JOIN ticket_feedback f ON f.ticket_id = t.id
		WHERE t.guild_id = $1 AND t.opened_at >= `+since+` AND t.claimed_by IS NOT NULL
		GROUP BY t.claimed_by ORDER BY count(*) DESC LIMIT 10`, gid, days)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var st StaffStat
		var uid int64
		if err := rows.Scan(&uid, &st.Name, &st.Claimed, &st.Closed, &st.AvgRating); err != nil {
			return a, err
		}
		st.UserID = snowflake.ID(uid)
		a.Staff = append(a.Staff, st)
	}
	return a, rows.Err()
}
