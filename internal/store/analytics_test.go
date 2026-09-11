package store

import (
	"context"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestAnalytics(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	billing := newType(t, s, testGuild, "Billing")
	support := newType(t, s, testGuild, "Support")
	staff := snowflake.ID(100)

	open := func(n int, tt TicketType, channel snowflake.ID) Ticket {
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	a := open(1, billing, 9001)
	b := open(2, billing, 9002)
	open(3, support, 9003)

	// a: responded after 10 minutes, resolved after an hour, rated 4.
	s.pool.Exec(ctx, `UPDATE tickets SET opened_at = now() - interval '1 hour',
		first_response_at = now() - interval '50 minutes' WHERE id = $1`, a.ID)
	s.ClaimTicket(ctx, a.ID, staff, "staff")
	s.CloseTicket(ctx, a.ID, staff, "staff", "")
	if err := s.SetFeedbackRating(ctx, a.ID, 4); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SetFeedbackComment(ctx, a.ID, "quick, thanks"); !ok {
		t.Error("comment on rated ticket failed")
	}
	if ok, _ := s.SetFeedbackComment(ctx, b.ID, "no rating"); ok {
		t.Error("comment without a rating should report false")
	}
	s.ClaimTicket(ctx, b.ID, staff, "staff")

	got, err := s.Analytics(ctx, testGuild, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Daily) != 7 || got.Opened != 3 || got.Closed != 1 {
		t.Errorf("daily=%d opened=%d closed=%d", len(got.Daily), got.Opened, got.Closed)
	}
	today := got.Daily[6]
	if today.Opened != 3 || today.Closed != 1 {
		t.Errorf("today = %+v", today)
	}
	if got.FirstResponseMedianSec == nil || int(*got.FirstResponseMedianSec) != 600 {
		t.Errorf("first response median = %v, want 600", got.FirstResponseMedianSec)
	}
	if got.ResolutionMedianSec == nil || *got.ResolutionMedianSec < 3500 {
		t.Errorf("resolution median = %v, want ~3600", got.ResolutionMedianSec)
	}
	if got.RatingAvg == nil || *got.RatingAvg != 4 || got.RatingCount != 1 {
		t.Errorf("rating = %v (%d)", got.RatingAvg, got.RatingCount)
	}
	if len(got.ByType) != 2 || got.ByType[0].Name != "Billing" || got.ByType[0].Count != 2 {
		t.Errorf("by type = %+v", got.ByType)
	}
	if len(got.Staff) != 1 || got.Staff[0].Claimed != 2 || got.Staff[0].Closed != 1 || *got.Staff[0].AvgRating != 4 {
		t.Errorf("staff = %+v", got.Staff)
	}

	// A guild with no tickets still gets a full, zero-filled series.
	seedGuild(t, s, 4001)
	empty, err := s.Analytics(ctx, 4001, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Daily) != 30 || empty.RatingAvg != nil || empty.FirstResponseMedianSec != nil {
		t.Errorf("empty analytics = %+v", empty)
	}
}
