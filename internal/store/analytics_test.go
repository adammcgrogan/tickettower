package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestAnalytics(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	billing := newType(t, s, testGuild, "Billing")
	support := newType(t, s, testGuild, "Support")
	staff := snowflake.ID(100)
	opener := snowflake.ID(42)

	open := func(n int, tt TicketType, channel snowflake.ID) Ticket {
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: opener, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	msgID := snowflake.ID(1)
	say := func(tk Ticket, author snowflake.ID) {
		msgID++
		err := s.InsertTicketMessage(ctx, TicketMessage{ID: msgID, TicketID: tk.ID, AuthorID: author,
			AuthorName: "someone", AuthorStaff: author == staff, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A reply the bot posted for a staff member, as the dashboard does.
	sayFor := func(tk Ticket, by snowflake.ID, name string) {
		msgID++
		err := s.InsertTicketMessage(ctx, TicketMessage{ID: msgID, TicketID: tk.ID, AuthorID: 7, AuthorName: "Ticket Tower",
			AuthorBot: true, SentBy: &by, AuthorStaff: true,
			Embeds: []Embed{{Author: &EmbedAuthor{Name: name}, Description: "hi"}}, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
	}
	a := open(1, billing, 9001)
	b := open(2, billing, 9002)
	c := open(3, support, 9003)
	d := open(4, billing, 9004)

	// a: responded after 10 minutes with one team message, resolved after an
	// hour by staff, rated 4.
	s.pool.Exec(ctx, `UPDATE tickets SET opened_at = now() - interval '1 hour',
		first_response_at = now() - interval '50 minutes' WHERE id = $1`, a.ID)
	say(a, opener)
	sayFor(a, staff, "staff")
	say(a, opener)
	say(a, 77) // a friend added with /ticket add: the member's side, not the team's
	s.ClaimTicket(ctx, a.ID, staff, "staff")
	s.CloseTicket(ctx, a.ID, staff, "staff", "Resolved")
	if err := s.SetFeedbackRating(ctx, a.ID, 4); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SetFeedbackComment(ctx, a.ID, "quick, thanks"); !ok {
		t.Error("comment on rated ticket failed")
	}
	if ok, _ := s.SetFeedbackComment(ctx, b.ID, "no rating"); ok {
		t.Error("comment without a rating should report false")
	}
	// b: claimed and still open. c: closed by the member, with the same reason
	// spelled differently, after having been reopened once. d: auto-closed.
	s.ClaimTicket(ctx, b.ID, staff, "staff")
	s.CloseTicket(ctx, c.ID, opener, "adam", " resolved ")
	s.pool.Exec(ctx, `UPDATE tickets SET reopened_at = now() - interval '30 minutes' WHERE id = $1`, c.ID)
	s.pool.Exec(ctx, `UPDATE tickets SET status = 'closed', closed_at = now(), auto_closed = true,
		close_reason = 'No activity for 1 day' WHERE id = $1`, d.ID)

	if err := s.RecordAnswerDeflection(ctx, testGuild, billing.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAnswerDeflection(ctx, testGuild, support.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.Analytics(ctx, testGuild, AnalyticsQuery{Days: 7})
	if err != nil {
		t.Fatal(err)
	}
	sm := got.Summary
	if len(got.Series) != 7 || got.Bucket != "day" || sm.Opened != 4 || sm.Closed != 3 {
		t.Errorf("series=%d bucket=%s opened=%d closed=%d", len(got.Series), got.Bucket, sm.Opened, sm.Closed)
	}
	if today := got.Series[6]; today.Opened != 4 || today.Closed != 3 || today.Backlog != 1 {
		t.Errorf("today = %+v", today)
	}
	if got.OpenNow != 1 {
		t.Errorf("open now = %d", got.OpenNow)
	}
	if sm.FirstResponseMedianSec == nil || int(*sm.FirstResponseMedianSec) != 600 {
		t.Errorf("first response median = %v, want 600", sm.FirstResponseMedianSec)
	}
	if sm.RatingAvg == nil || *sm.RatingAvg != 4 || sm.RatingCount != 1 || got.Ratings[3] != 1 {
		t.Errorf("rating = %v (%d) %v", sm.RatingAvg, sm.RatingCount, got.Ratings)
	}
	if sm.ClosedUnanswered != 2 || sm.Transcripts != 1 || sm.TeamMessages != 1 || sm.MemberMessages != 3 || sm.OneTouch != 1 {
		t.Errorf("summary = %+v", sm)
	}
	if sm.AnswersDeflected != 2 {
		t.Errorf("answers deflected = %d, want 2", sm.AnswersDeflected)
	}
	// a and b were both claimed a moment after opening; a was backdated an
	// hour, so the median sits around half that.
	if sm.ClaimMedianSec == nil || *sm.ClaimMedianSec < 1500 || *sm.ClaimMedianSec > 2000 {
		t.Errorf("claim median = %v, want ~1800", sm.ClaimMedianSec)
	}
	if sm.Reopened != 1 {
		t.Errorf("reopened = %d, want 1", sm.Reopened)
	}
	if got.BacklogAgeMedianSec == nil {
		t.Error("backlog age median = nil, want a value while a ticket is open")
	}
	if got.Closures != (Closures{Team: 1, Member: 1, Auto: 1}) {
		t.Errorf("closures = %+v", got.Closures)
	}
	if len(got.CloseReasons) != 1 || got.CloseReasons[0].Count != 2 || strings.ToLower(got.CloseReasons[0].Reason) != "resolved" {
		t.Errorf("close reasons = %+v", got.CloseReasons)
	}
	heat := 0
	for _, day := range got.Heatmap {
		for _, n := range day {
			heat += n
		}
	}
	if heat != 4 || got.Channels != 4 || got.Threads != 0 {
		t.Errorf("heatmap total = %d, channels = %d", heat, got.Channels)
	}
	if len(got.ByType) != 2 || got.ByType[0].Name != "Billing" || got.ByType[0].Opened != 3 ||
		got.ByType[0].RatingCount != 1 || *got.ByType[0].TypeID != billing.ID {
		t.Errorf("by type = %+v", got.ByType)
	}
	if len(got.Staff) != 1 || got.Staff[0].Claimed != 2 || got.Staff[0].Closed != 1 || got.Staff[0].Replies != 1 ||
		got.Staff[0].Tickets != 1 || *got.Staff[0].AvgRating != 4 || got.Staff[0].Name != "staff" {
		t.Errorf("staff = %+v", got.Staff)
	}
	if got.Previous == nil || got.Previous.Opened != 0 {
		t.Errorf("previous = %+v", got.Previous)
	}

	// Filtering by type only counts that type's tickets.
	only, err := s.Analytics(ctx, testGuild, AnalyticsQuery{Days: 7, TypeID: &support.ID})
	if err != nil {
		t.Fatal(err)
	}
	if only.Summary.Opened != 1 || only.OpenNow != 0 || len(only.ByType) != 1 || len(only.Staff) != 0 {
		t.Errorf("filtered = %+v", only)
	}
	if only.Summary.AnswersDeflected != 1 {
		t.Errorf("filtered answers deflected = %d, want 1", only.Summary.AnswersDeflected)
	}

	// All time has no previous window and still shows at least a week.
	all, err := s.Analytics(ctx, testGuild, AnalyticsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Previous != nil || all.Days != 0 || len(all.Series) != 7 || all.Summary.Opened != 4 {
		t.Errorf("all time: previous=%v days=%d series=%d opened=%d", all.Previous, all.Days, len(all.Series), all.Summary.Opened)
	}

	// A year is bucketed by week.
	year, err := s.Analytics(ctx, testGuild, AnalyticsQuery{Days: 365})
	if err != nil {
		t.Fatal(err)
	}
	if year.Bucket != "week" || len(year.Series) < 53 || year.Series[len(year.Series)-1].Opened != 4 {
		t.Errorf("year: bucket=%s series=%d", year.Bucket, len(year.Series))
	}

	// A guild with no tickets still gets a full, zero-filled series.
	seedGuild(t, s, 4001)
	empty, err := s.Analytics(ctx, 4001, AnalyticsQuery{Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Series) != 30 || empty.Summary.RatingAvg != nil || empty.Summary.FirstResponseMedianSec != nil {
		t.Errorf("empty analytics = %+v", empty)
	}
}
