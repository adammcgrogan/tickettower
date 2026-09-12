package store

import (
	"context"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestHoldAndWaitClock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")

	tk := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
		ChannelID: 7101, OpenerID: 42, OpenerName: "member"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}
	if tk.WaitingSince != nil {
		t.Errorf("a ticket opened without a form isn't waiting on staff yet: %v", tk.WaitingSince)
	}
	withForm := Ticket{GuildID: testGuild, Number: 2, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
		ChannelID: 7102, OpenerID: 42, OpenerName: "member", WaitingOnStaff: true}
	if err := s.CreateTicket(ctx, &withForm); err != nil {
		t.Fatal(err)
	}
	if withForm.WaitingSince == nil {
		t.Error("a ticket opened with answers starts the wait clock")
	}

	now := time.Now()
	// The member writes: the clock starts and keeps its start on later messages.
	if err := s.RecordActivity(ctx, tk.ID, now, true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordActivity(ctx, tk.ID, now.Add(time.Minute), true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTicket(ctx, tk.ID)
	if got.WaitingSince == nil || !got.WaitingSince.Equal(now.Truncate(time.Microsecond)) {
		t.Errorf("waiting_since = %v, want %v", got.WaitingSince, now)
	}
	// Staff reply: the clock stops.
	if err := s.RecordActivity(ctx, tk.ID, now.Add(2*time.Minute), false); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetTicket(ctx, tk.ID); got.WaitingSince != nil || got.WaitingOnStaff {
		t.Errorf("after a staff reply = %+v", got)
	}

	// Hold and resume.
	if ok, err := s.HoldTicket(ctx, tk.ID, "Waiting on the payment provider"); err != nil || !ok {
		t.Fatalf("hold = %v, %v", ok, err)
	}
	if ok, _ := s.HoldTicket(ctx, tk.ID, "again"); ok {
		t.Error("a ticket can't be held twice")
	}
	got, _ = s.GetTicket(ctx, tk.ID)
	if !got.OnHold || got.HoldReason != "Waiting on the payment provider" {
		t.Errorf("held ticket = %+v", got)
	}
	// The member writing takes it off hold and starts the wait.
	if err := s.RecordActivity(ctx, tk.ID, now.Add(3*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetTicket(ctx, tk.ID); got.OnHold || got.HoldReason != "" || got.WaitingSince == nil {
		t.Errorf("after the member writes = %+v", got)
	}
	if ok, _ := s.ResumeTicket(ctx, tk.ID, now); ok {
		t.Error("resume on a ticket that isn't held reports false")
	}
	// Staff messages don't lift a hold.
	s.HoldTicket(ctx, tk.ID, "x")
	s.RecordActivity(ctx, tk.ID, now.Add(4*time.Minute), false)
	if got, _ = s.GetTicket(ctx, tk.ID); !got.OnHold {
		t.Error("a staff message lifted the hold")
	}
	if ok, err := s.ResumeTicket(ctx, tk.ID, now); err != nil || !ok {
		t.Fatalf("resume = %v, %v", ok, err)
	}
}

func TestReminders(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")
	mins := 60
	tt.ReminderMinutes = &mins
	tt.ReminderRepeat = true
	if err := s.UpdateTicketType(ctx, tt); err != nil {
		t.Fatal(err)
	}
	once := newType(t, s, testGuild, "Once")
	once.ReminderMinutes = &mins
	if err := s.UpdateTicketType(ctx, once); err != nil {
		t.Fatal(err)
	}
	quiet := newType(t, s, testGuild, "No reminders")

	start := time.Now().Add(-3 * time.Hour)
	open := func(typ TicketType, channel snowflake.ID) Ticket {
		t.Helper()
		n, _ := s.NextTicketNumber(ctx, testGuild)
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &typ.ID, TypeName: typ.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "member"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		// The member wrote at start.
		if err := s.RecordActivity(ctx, tk.ID, start, true); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	repeating := open(tt, 7201)
	single := open(once, 7202)
	open(quiet, 7203)
	held := open(tt, 7204)
	s.HoldTicket(ctx, held.ID, "vendor")
	answered := open(tt, 7205)
	s.RecordActivity(ctx, answered.ID, start.Add(time.Minute), false)

	ids := func(now time.Time) []int64 {
		t.Helper()
		due, err := s.TicketsToRemind(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, w := range due {
			out = append(out, w.ID)
		}
		return out
	}
	if got := ids(start.Add(59 * time.Minute)); len(got) != 0 {
		t.Errorf("reminded before the hour: %v", got)
	}
	got := ids(start.Add(61 * time.Minute))
	if len(got) != 2 || got[0] != repeating.ID || got[1] != single.ID {
		t.Fatalf("due after an hour = %v, want %d and %d", got, repeating.ID, single.ID)
	}
	for _, id := range got {
		if ok, err := s.MarkReminded(ctx, id, 60, start.Add(61*time.Minute)); err != nil || !ok {
			t.Fatalf("MarkReminded(%d) = %v, %v", id, ok, err)
		}
	}
	// Not again straight away.
	if got := ids(start.Add(90 * time.Minute)); len(got) != 0 {
		t.Errorf("reminded again too soon: %v", got)
	}
	// After another hour only the repeating type comes back.
	if got := ids(start.Add(125 * time.Minute)); len(got) != 1 || got[0] != repeating.ID {
		t.Errorf("second round = %v, want only %d", got, repeating.ID)
	}
	// A staff reply ends it.
	s.RecordActivity(ctx, repeating.ID, start.Add(126*time.Minute), false)
	if got := ids(start.Add(200 * time.Minute)); len(got) != 0 {
		t.Errorf("reminded after a staff reply: %v", got)
	}
	// The wait clock survives a second member message, so the next reminder
	// counts from when they first asked, not from their nudge.
	s.RecordActivity(ctx, single.ID, start.Add(100*time.Minute), true)
	w, _ := s.TicketsToRemind(ctx, start.Add(61*time.Minute))
	_ = w
	got2, _ := s.GetTicket(ctx, single.ID)
	if got2.WaitingSince == nil || !got2.WaitingSince.Equal(start.Truncate(time.Microsecond)) {
		t.Errorf("waiting_since moved to %v", got2.WaitingSince)
	}
}
