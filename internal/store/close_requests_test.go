package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestCloseRequests(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")
	open := func(channel snowflake.ID) Ticket {
		t.Helper()
		tk := Ticket{GuildID: testGuild, Number: int(channel), TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	timed, answered, forever := open(1), open(2), open(3)
	now := time.Now()
	in := now.Add(time.Hour)

	for _, tk := range []Ticket{timed, answered} {
		if ok, err := s.RequestClose(ctx, CloseRequest{TicketID: tk.ID, By: 100, ByName: "staff", Reason: "Resolved", ClosesAt: &in}); err != nil || !ok {
			t.Fatalf("request: ok=%v err=%v", ok, err)
		}
	}
	if ok, _ := s.RequestClose(ctx, CloseRequest{TicketID: forever.ID, By: 100, ByName: "staff"}); !ok {
		t.Fatal("open-ended request failed")
	}
	r, err := s.GetCloseRequest(ctx, timed.ID)
	if err != nil || r.By != 100 || r.Reason != "Resolved" || r.ClosesAt == nil {
		t.Fatalf("request = %+v, err %v", r, err)
	}

	// The member writing again withdraws the request.
	if err := s.RecordActivity(ctx, answered.ID, now, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCloseRequest(ctx, answered.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("request survived the member's message: %v", err)
	}
	// A staff message doesn't.
	if err := s.RecordActivity(ctx, timed.ID, now, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCloseRequest(ctx, timed.ID); err != nil {
		t.Errorf("request lost on a staff message: %v", err)
	}

	// Only timed requests come due, and only once their time is up.
	if due, _ := s.CloseRequestsDue(ctx, now); len(due) != 0 {
		t.Errorf("%d due early", len(due))
	}
	due, err := s.CloseRequestsDue(ctx, in)
	if err != nil || len(due) != 1 || due[0].TicketID != timed.ID {
		t.Fatalf("due = %+v, err %v", due, err)
	}
	if ok, _ := s.CloseUnansweredRequest(ctx, timed.ID, now); ok {
		t.Error("closed before the request was due")
	}
	if ok, err := s.CloseUnansweredRequest(ctx, timed.ID, in); err != nil || !ok {
		t.Fatalf("close unanswered: ok=%v err=%v", ok, err)
	}
	got, _ := s.GetTicket(ctx, timed.ID)
	if got.Status != StatusClosed || got.ClosedBy == nil || *got.ClosedBy != 100 || got.CloseReason != "Resolved" {
		t.Errorf("closed ticket = %+v", got)
	}

	// Keeping it open clears the request; clearing twice reports false.
	if ok, _ := s.ClearCloseRequest(ctx, forever.ID); !ok {
		t.Error("clear failed")
	}
	if ok, _ := s.ClearCloseRequest(ctx, forever.ID); ok {
		t.Error("second clear should report false")
	}
}
