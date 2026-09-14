package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
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

// Feedback is the opener's rating and comment for a ticket.
type Feedback struct {
	Rating  int
	Comment string
}

// GetFeedback returns a ticket's feedback, or ErrNotFound if it hasn't been
// rated.
func (s *Store) GetFeedback(ctx context.Context, ticketID int64) (Feedback, error) {
	var f Feedback
	err := s.pool.QueryRow(ctx, `SELECT rating, comment FROM ticket_feedback WHERE ticket_id = $1`, ticketID).
		Scan(&f.Rating, &f.Comment)
	return f, notFound(err)
}

// AnalyticsQuery picks the reporting window and, optionally, one ticket type.
type AnalyticsQuery struct {
	Days   int    // 0 means all time
	TypeID *int64 // nil means every type
}

// Summary holds the headline figures for one window. Openings, first
// responses and the hour breakdowns count tickets opened in the window;
// closes, resolution times and message stats count tickets closed in it.
type Summary struct {
	Opened                 int      `json:"opened"`
	Closed                 int      `json:"closed"`
	FirstResponseMedianSec *float64 `json:"first_response_median_seconds"`
	ResolutionMedianSec    *float64 `json:"resolution_median_seconds"`
	RatingAvg              *float64 `json:"rating_avg"`
	RatingCount            int      `json:"rating_count"`
	// TargetMeasured counts tickets opened in the window whose type has a
	// reply target; TargetMet is how many got their first reply in time.
	TargetMeasured int `json:"target_measured"`
	TargetMet      int `json:"target_met"`
	// Closed before anyone other than the opener replied.
	ClosedUnanswered int `json:"closed_unanswered"`
	// Message stats only cover closed tickets whose transcript is still kept.
	Transcripts    int `json:"transcripts"`
	TeamMessages   int `json:"team_messages"`
	MemberMessages int `json:"member_messages"`
	OneTouch       int `json:"one_touch"` // resolved with a single team message
}

type SeriesPoint struct {
	Date    string `json:"date"` // start of the bucket, YYYY-MM-DD in UTC
	Opened  int    `json:"opened"`
	Closed  int    `json:"closed"`
	Backlog int    `json:"backlog"` // open at the end of the bucket
}

type TypeStat struct {
	TypeID                 *int64   `json:"type_id"` // nil for deleted types
	Name                   string   `json:"name"`
	Emoji                  string   `json:"emoji"`
	Opened                 int      `json:"opened"`
	FirstResponseMedianSec *float64 `json:"first_response_median_seconds"`
	ResolutionMedianSec    *float64 `json:"resolution_median_seconds"`
	RatingAvg              *float64 `json:"rating_avg"`
	RatingCount            int      `json:"rating_count"`
	// AsksRating is false for types that don't ask for ratings, so a blank
	// rating can be explained. Deleted types count as asking.
	AsksRating bool `json:"asks_rating"`
	// TargetMeasured and TargetMet: tickets with a reply target, and how
	// many were answered within it.
	TargetMeasured int `json:"target_measured"`
	TargetMet      int `json:"target_met"`
}

type StaffStat struct {
	UserID    snowflake.ID `json:"user_id"`
	Name      string       `json:"name"`
	Claimed   int          `json:"claimed"`
	Closed    int          `json:"closed"`
	Replies   int          `json:"replies"`
	Tickets   int          `json:"tickets"` // tickets they replied in
	AvgRating *float64     `json:"avg_rating"`
}

type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// Closures splits the window's closes by who (or what) closed the ticket.
type Closures struct {
	Team    int `json:"team"`
	Member  int `json:"member"`
	Auto    int `json:"auto"`
	Deleted int `json:"deleted"` // the channel was deleted outside the bot
}

type Analytics struct {
	Days     int      `json:"days"`
	From     string   `json:"from"`   // first day of the window, YYYY-MM-DD
	Bucket   string   `json:"bucket"` // day, week or month
	Summary  Summary  `json:"summary"`
	Previous *Summary `json:"previous"` // the window before; nil for all time

	OpenNow    int `json:"open_now"`
	WaitingNow int `json:"waiting_now"` // open, waiting on the team and not on hold
	OnHoldNow  int `json:"on_hold_now"`
	OverdueNow int `json:"overdue_now"` // waiting longer than the type's reply target

	Series         []SeriesPoint `json:"series"`
	Heatmap        [][]int       `json:"heatmap"`          // [weekday, Sunday first][hour], UTC
	ResponseByHour []*float64    `json:"response_by_hour"` // median first response by hour opened
	Ratings        []int         `json:"ratings"`          // count of 1 to 5 star ratings
	Closures       Closures      `json:"closures"`
	CloseReasons   []ReasonCount `json:"close_reasons"`
	Threads        int           `json:"threads"`
	Channels       int           `json:"channels"`
	ByType         []TypeStat    `json:"by_type"`
	Staff          []StaffStat   `json:"staff"`
}

// Every window query takes the same arguments: $1 guild, $2 and $3 the
// window [from, to), $4 an optional ticket type.
const (
	inScope  = `t.guild_id = $1 AND ($4::bigint IS NULL OR t.ticket_type_id = $4)`
	openedIn = ` AND t.opened_at >= $2 AND t.opened_at < $3`
	closedIn = ` AND t.closed_at >= $2 AND t.closed_at < $3`
	// A team message is one the bot marked as staff when it was captured
	// (a support role or server manager, or a reply posted for a staff
	// member), which is the same rule it uses for the first response. Bots
	// and everyone else in the ticket are the member's side.
	teamMsg   = `m.author_staff`
	memberMsg = `NOT m.author_staff AND NOT m.author_bot`
	// teamMember is who a team message counts for, and their name: the staff
	// member behind a bot-posted reply is named in the embed's author.
	teamMember     = `COALESCE(m.sent_by, m.author_id)`
	teamMemberName = `COALESCE(m.embeds->0->'author'->>'name', m.author_name)`
)

// Analytics summarises a guild's tickets over the requested window.
func (s *Store) Analytics(ctx context.Context, guildID snowflake.ID, q AnalyticsQuery) (Analytics, error) {
	gid := int64(guildID)
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)

	from := today.AddDate(0, 0, 1-q.Days)
	if q.Days <= 0 {
		// All time starts at the first ticket, but always shows at least a week.
		var first *time.Time
		err := s.pool.QueryRow(ctx, `SELECT min(opened_at) FROM tickets t WHERE `+
			`t.guild_id = $1 AND ($2::bigint IS NULL OR t.ticket_type_id = $2)`, gid, q.TypeID).Scan(&first)
		if err != nil {
			return Analytics{}, err
		}
		from = today.AddDate(0, 0, -6)
		if first != nil && first.Before(from) {
			from = first.UTC().Truncate(24 * time.Hour)
		}
	}
	bucket := "day"
	if span := today.Sub(from).Hours() / 24; span > 731 {
		bucket = "month"
	} else if span > 92 {
		bucket = "week"
	}

	a := Analytics{
		Days: max(q.Days, 0), From: from.Format(time.DateOnly), Bucket: bucket,
		Series: []SeriesPoint{}, CloseReasons: []ReasonCount{}, ByType: []TypeStat{}, Staff: []StaffStat{},
		Heatmap: make([][]int, 7), ResponseByHour: make([]*float64, 24),
	}
	for i := range a.Heatmap {
		a.Heatmap[i] = make([]int, 24)
	}
	args := []any{gid, from, now, q.TypeID}

	var err error
	if a.Summary, a.Ratings, err = s.summary(ctx, args); err != nil {
		return a, err
	}
	if q.Days > 0 {
		prev, _, err := s.summary(ctx, []any{gid, from.AddDate(0, 0, -q.Days), from, q.TypeID})
		if err != nil {
			return a, err
		}
		a.Previous = &prev
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE t.status = 'open'),
		       count(*) FILTER (WHERE t.status = 'open' AND t.waiting_on_staff AND NOT t.on_hold),
		       count(*) FILTER (WHERE t.status = 'open' AND t.on_hold),
		       count(*) FILTER (WHERE t.status = 'open' AND t.waiting_on_staff AND NOT t.on_hold
		                          AND tt.reply_target_minutes IS NOT NULL
		                          AND t.waiting_since + make_interval(mins => tt.reply_target_minutes) <= $3)
		FROM tickets t LEFT JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE t.guild_id = $1 AND ($2::bigint IS NULL OR t.ticket_type_id = $2)`,
		gid, q.TypeID, now).Scan(&a.OpenNow, &a.WaitingNow, &a.OnHoldNow, &a.OverdueNow)
	if err != nil {
		return a, err
	}

	// Buckets are clamped to the window, so a week that starts before it
	// only counts the days inside. Backlog is measured at the bucket's end,
	// or now for the current one.
	rows, err := s.pool.Query(ctx, `
		SELECT to_char(b AT TIME ZONE 'UTC', 'YYYY-MM-DD'),
		       (SELECT count(*) FROM tickets t WHERE `+inScope+`
		           AND t.opened_at >= GREATEST(b, $2) AND t.opened_at < LEAST(b + step, $3)),
		       (SELECT count(*) FROM tickets t WHERE `+inScope+`
		           AND t.closed_at >= GREATEST(b, $2) AND t.closed_at < LEAST(b + step, $3)),
		       (SELECT count(*) FROM tickets t WHERE `+inScope+`
		           AND t.opened_at < LEAST(b + step, $3) AND (t.closed_at IS NULL OR t.closed_at >= LEAST(b + step, $3)))
		FROM (SELECT ('1 ' || $5::text)::interval AS step) i,
		     generate_series(date_trunc($5::text, $2::timestamptz, 'UTC'), $3::timestamptz, i.step) b
		ORDER BY b`, append(args, bucket)...)
	err = eachRow(rows, err, func() error {
		var p SeriesPoint
		err := rows.Scan(&p.Date, &p.Opened, &p.Closed, &p.Backlog)
		a.Series = append(a.Series, p)
		return err
	})
	if err != nil {
		return a, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT extract(dow FROM t.opened_at AT TIME ZONE 'UTC')::int, extract(hour FROM t.opened_at AT TIME ZONE 'UTC')::int,
		       count(*)
		FROM tickets t WHERE `+inScope+openedIn+` GROUP BY 1, 2`, args...)
	err = eachRow(rows, err, func() error {
		var dow, hour, n int
		err := rows.Scan(&dow, &hour, &n)
		a.Heatmap[dow][hour] = n
		return err
	})
	if err != nil {
		return a, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT extract(hour FROM t.opened_at AT TIME ZONE 'UTC')::int,
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.first_response_at - t.opened_at))
		FROM tickets t WHERE `+inScope+openedIn+` AND t.first_response_at IS NOT NULL GROUP BY 1`, args...)
	err = eachRow(rows, err, func() error {
		var hour int
		var median float64
		err := rows.Scan(&hour, &median)
		a.ResponseByHour[hour] = &median
		return err
	})
	if err != nil {
		return a, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE t.auto_closed),
		       count(*) FILTER (WHERE t.closed_by IS NULL AND NOT t.auto_closed),
		       count(*) FILTER (WHERE t.closed_by = t.opener_id),
		       count(*) FILTER (WHERE t.closed_by <> t.opener_id)
		FROM tickets t WHERE `+inScope+closedIn, args...).
		Scan(&a.Closures.Auto, &a.Closures.Deleted, &a.Closures.Member, &a.Closures.Team)
	if err != nil {
		return a, err
	}

	// Reasons are grouped ignoring case and spacing, and shown in their most
	// common spelling. Automatic closes are covered by Closures instead.
	rows, err = s.pool.Query(ctx, `
		SELECT mode() WITHIN GROUP (ORDER BY btrim(t.close_reason)), count(*)
		FROM tickets t WHERE `+inScope+closedIn+` AND t.closed_by IS NOT NULL AND btrim(t.close_reason) <> ''
		GROUP BY lower(btrim(t.close_reason)) ORDER BY count(*) DESC, 1 LIMIT 8`, args...)
	err = eachRow(rows, err, func() error {
		var r ReasonCount
		err := rows.Scan(&r.Reason, &r.Count)
		a.CloseReasons = append(a.CloseReasons, r)
		return err
	})
	if err != nil {
		return a, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE t.mode = 'thread'), count(*) FILTER (WHERE t.mode = 'channel')
		FROM tickets t WHERE `+inScope+openedIn, args...).Scan(&a.Threads, &a.Channels)
	if err != nil {
		return a, err
	}

	// Types are grouped by ID so a renamed type stays one row; deleted types
	// fall back to the name the ticket was opened under.
	rows, err = s.pool.Query(ctx, `
		SELECT t.ticket_type_id, COALESCE(max(tt.name), max(t.type_name)), COALESCE(max(tt.emoji), ''), count(*),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.first_response_at - t.opened_at))
		           FILTER (WHERE t.first_response_at IS NOT NULL),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.closed_at - t.opened_at))
		           FILTER (WHERE t.closed_at IS NOT NULL),
		       avg(f.rating)::float8, count(f.rating), COALESCE(bool_or(tt.ask_rating), true),
		       count(*) FILTER (WHERE tt.reply_target_minutes IS NOT NULL),
		       count(*) FILTER (WHERE tt.reply_target_minutes IS NOT NULL AND t.first_response_at IS NOT NULL
		                          AND t.first_response_at - t.opened_at <= make_interval(mins => tt.reply_target_minutes))
		FROM tickets t
		LEFT JOIN ticket_types tt ON tt.id = t.ticket_type_id
		LEFT JOIN ticket_feedback f ON f.ticket_id = t.id
		WHERE `+inScope+openedIn+`
		GROUP BY t.ticket_type_id, CASE WHEN t.ticket_type_id IS NULL THEN t.type_name END
		ORDER BY count(*) DESC, 2 LIMIT 25`, args...)
	err = eachRow(rows, err, func() error {
		var ts TypeStat
		err := rows.Scan(&ts.TypeID, &ts.Name, &ts.Emoji, &ts.Opened, &ts.FirstResponseMedianSec,
			&ts.ResolutionMedianSec, &ts.RatingAvg, &ts.RatingCount, &ts.AsksRating, &ts.TargetMeasured, &ts.TargetMet)
		a.ByType = append(a.ByType, ts)
		return err
	})
	if err != nil {
		return a, err
	}

	// The team table joins three views of the window: tickets claimed (and
	// how they were rated), tickets closed, and replies sent.
	rows, err = s.pool.Query(ctx, `
		WITH claims AS (
			SELECT t.claimed_by AS uid, max(t.claimed_by_name) AS name, count(*) AS n, avg(f.rating)::float8 AS rating
			FROM tickets t LEFT JOIN ticket_feedback f ON f.ticket_id = t.id
			WHERE `+inScope+openedIn+` AND t.claimed_by IS NOT NULL GROUP BY 1
		), closes AS (
			SELECT t.closed_by AS uid, max(t.closed_by_name) AS name, count(*) AS n
			FROM tickets t WHERE `+inScope+closedIn+` AND t.closed_by <> t.opener_id GROUP BY 1
		), replies AS (
			SELECT `+teamMember+` AS uid, max(`+teamMemberName+`) AS name, count(*) AS n,
			       count(DISTINCT m.ticket_id) AS tickets
			FROM tickets t JOIN ticket_messages m ON m.ticket_id = t.id
			WHERE `+inScope+openedIn+` AND `+teamMsg+` GROUP BY 1
		), people AS (
			SELECT uid FROM claims UNION SELECT uid FROM closes UNION SELECT uid FROM replies
		)
		SELECT p.uid, COALESCE(r.name, c.name, x.name, ''), COALESCE(c.n, 0), COALESCE(x.n, 0),
		       COALESCE(r.n, 0), COALESCE(r.tickets, 0), c.rating
		FROM people p
		LEFT JOIN claims c ON c.uid = p.uid
		LEFT JOIN closes x ON x.uid = p.uid
		LEFT JOIN replies r ON r.uid = p.uid
		ORDER BY COALESCE(r.n, 0) + COALESCE(c.n, 0) + COALESCE(x.n, 0) DESC, 2 LIMIT 15`, args...)
	err = eachRow(rows, err, func() error {
		var st StaffStat
		var uid int64
		err := rows.Scan(&uid, &st.Name, &st.Claimed, &st.Closed, &st.Replies, &st.Tickets, &st.AvgRating)
		st.UserID = snowflake.ID(uid)
		a.Staff = append(a.Staff, st)
		return err
	})
	return a, err
}

// summary computes the headline figures for the window in args, plus the
// count of each star rating given in it.
func (s *Store) summary(ctx context.Context, args []any) (Summary, []int, error) {
	var sm Summary
	ratings := make([]int, 5)
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE t.opened_at >= $2 AND t.opened_at < $3),
		       count(*) FILTER (WHERE t.closed_at >= $2 AND t.closed_at < $3),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.first_response_at - t.opened_at))
		           FILTER (WHERE t.opened_at >= $2 AND t.opened_at < $3 AND t.first_response_at IS NOT NULL),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM t.closed_at - t.opened_at))
		           FILTER (WHERE t.closed_at >= $2 AND t.closed_at < $3),
		       count(*) FILTER (WHERE t.closed_at >= $2 AND t.closed_at < $3 AND t.first_response_at IS NULL),
		       count(*) FILTER (WHERE t.opened_at >= $2 AND t.opened_at < $3 AND tt.reply_target_minutes IS NOT NULL),
		       count(*) FILTER (WHERE t.opened_at >= $2 AND t.opened_at < $3 AND tt.reply_target_minutes IS NOT NULL
		                          AND t.first_response_at IS NOT NULL
		                          AND t.first_response_at - t.opened_at <= make_interval(mins => tt.reply_target_minutes))
		FROM tickets t LEFT JOIN ticket_types tt ON tt.id = t.ticket_type_id WHERE `+inScope, args...).
		Scan(&sm.Opened, &sm.Closed, &sm.FirstResponseMedianSec, &sm.ResolutionMedianSec, &sm.ClosedUnanswered,
			&sm.TargetMeasured, &sm.TargetMet)
	if err != nil {
		return sm, ratings, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(team), 0), COALESCE(sum(member), 0), count(*) FILTER (WHERE team = 1)
		FROM (
			SELECT count(*) FILTER (WHERE `+teamMsg+`) AS team, count(*) FILTER (WHERE `+memberMsg+`) AS member
			FROM tickets t JOIN ticket_messages m ON m.ticket_id = t.id
			WHERE `+inScope+closedIn+` GROUP BY t.id
		) x`, args...).Scan(&sm.Transcripts, &sm.TeamMessages, &sm.MemberMessages, &sm.OneTouch)
	if err != nil {
		return sm, ratings, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT f.rating, count(*) FROM ticket_feedback f JOIN tickets t ON t.id = f.ticket_id
		WHERE `+inScope+` AND f.created_at >= $2 AND f.created_at < $3 GROUP BY 1`, args...)
	total := 0
	err = eachRow(rows, err, func() error {
		var rating, n int
		if err := rows.Scan(&rating, &n); err != nil {
			return err
		}
		ratings[rating-1] = n
		sm.RatingCount += n
		total += rating * n
		return nil
	})
	if sm.RatingCount > 0 {
		avg := float64(total) / float64(sm.RatingCount)
		sm.RatingAvg = &avg
	}
	return sm, ratings, err
}

// eachRow calls fn for every row, closing rows and returning the first error.
func eachRow(rows pgx.Rows, err error, fn func() error) error {
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(); err != nil {
			return err
		}
	}
	return rows.Err()
}
