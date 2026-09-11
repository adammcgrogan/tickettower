package store

import (
	"context"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestAutoClose(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)

	tt := newType(t, s, testGuild, "Support")
	hours := 24
	tt.AutoCloseHours = &hours
	if err := s.UpdateTicketType(ctx, tt); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTicketType(ctx, testGuild, tt.ID); got.AutoCloseHours == nil || *got.AutoCloseHours != 24 {
		t.Fatalf("auto_close_hours = %v", got.AutoCloseHours)
	}
	off := newType(t, s, testGuild, "No auto-close")

	open := func(typ TicketType, channel snowflake.ID, waiting bool) Ticket {
		t.Helper()
		n, _ := s.NextTicketNumber(ctx, testGuild)
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &typ.ID, TypeName: typ.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "adam", WaitingOnStaff: waiting}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	quiet := open(tt, 9001, false)
	answered := open(tt, 9002, true) // opened with a form, so staff owe a reply
	open(off, 9003, false)
	start := quiet.LastActivityAt
	at := func(h float64) time.Time { return start.Add(time.Duration(h * float64(time.Hour))) }

	ids := func(its []InactiveTicket, err error) []int64 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, it := range its {
			out = append(out, it.ID)
		}
		return out
	}

	// A 24 hour window warns 6 hours ahead, so at 18 hours.
	if got := ids(s.TicketsToWarn(ctx, at(17.9))); len(got) != 0 {
		t.Errorf("warned too early: %v", got)
	}
	due, err := s.TicketsToWarn(ctx, at(18))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != quiet.ID || due[0].Hours != 24 || due[0].ChannelID != 9001 || !due[0].CloseAt.Equal(at(24)) {
		t.Fatalf("to warn = %+v", due)
	}
	if ok, _ := s.MarkAutoCloseWarned(ctx, quiet.ID, at(18)); !ok {
		t.Fatal("mark warned failed")
	}
	if ok, _ := s.MarkAutoCloseWarned(ctx, quiet.ID, at(18)); ok {
		t.Error("second warning should report false")
	}
	if got := ids(s.TicketsToWarn(ctx, at(19))); len(got) != 0 {
		t.Errorf("warned twice: %v", got)
	}

	// Nothing closes before the window ends.
	if got := ids(s.TicketsToAutoClose(ctx, at(23.9))); len(got) != 0 {
		t.Errorf("closed too early: %v", got)
	}
	if ok, _ := s.AutoCloseTicket(ctx, quiet.ID, "idle", at(23.9)); ok {
		t.Error("AutoCloseTicket closed a ticket that wasn't due")
	}

	// A staff reply restarts the clock and cancels the warning.
	if err := s.RecordActivity(ctx, quiet.ID, at(20), false); err != nil {
		t.Fatal(err)
	}
	if got := ids(s.TicketsToAutoClose(ctx, at(30))); len(got) != 0 {
		t.Errorf("closed after a reply: %v", got)
	}
	if got := ids(s.TicketsToWarn(ctx, at(38))); len(got) != 1 || got[0] != quiet.ID {
		t.Errorf("to warn after reply = %v", got)
	}

	// A warning posted late still gives the member the full lead time.
	s.MarkAutoCloseWarned(ctx, quiet.ID, at(43))
	if got := ids(s.TicketsToAutoClose(ctx, at(44))); len(got) != 0 {
		t.Errorf("closed without the full warning period: %v", got)
	}
	if got := ids(s.TicketsToAutoClose(ctx, at(49))); len(got) != 1 {
		t.Fatalf("to close = %v", got)
	}
	if ok, err := s.AutoCloseTicket(ctx, quiet.ID, "No activity for 1 day", at(49)); !ok || err != nil {
		t.Fatalf("auto close = %v, %v", ok, err)
	}
	got, _ := s.GetTicket(ctx, quiet.ID)
	if got.Status != StatusClosed || got.CloseReason != "No activity for 1 day" || got.ClosedBy != nil || got.ClosedAt == nil {
		t.Errorf("closed ticket = %+v", got)
	}

	// Tickets waiting on staff never auto-close until staff reply.
	if got := ids(s.TicketsToWarn(ctx, at(1000))); len(got) != 0 {
		t.Errorf("warned a ticket waiting on staff: %v", got)
	}
	// (Activity never moves backwards, so this ticket's clock still starts
	// when it was opened, a moment after the first one.)
	s.RecordActivity(ctx, answered.ID, start, false)
	if got := ids(s.TicketsToWarn(ctx, answered.LastActivityAt.Add(18*time.Hour))); len(got) != 1 || got[0] != answered.ID {
		t.Errorf("after staff reply = %v", got)
	}
	s.RecordActivity(ctx, answered.ID, at(1), true)
	if got := ids(s.TicketsToWarn(ctx, at(1000))); len(got) != 0 {
		t.Errorf("after member reply = %v", got)
	}
}
