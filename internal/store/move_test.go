package store

import (
	"context"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestMoveTicket(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	general := newType(t, s, testGuild, "General")
	billing := newType(t, s, testGuild, "Billing")

	tk := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &general.ID, TypeName: general.Name, Mode: ModeChannel,
		ChannelID: 7101, OpenerID: 42, OpenerName: "adam"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}

	// Another guild can't move it.
	if ok, err := s.MoveTicket(ctx, snowflake.ID(9999), tk.ID, billing.ID, billing.Name); ok || err != nil {
		t.Errorf("other guild: ok = %v, err = %v", ok, err)
	}
	if ok, err := s.MoveTicket(ctx, testGuild, tk.ID, billing.ID, billing.Name); !ok || err != nil {
		t.Fatalf("move: ok = %v, err = %v", ok, err)
	}
	got, _ := s.GetTicket(ctx, tk.ID)
	if got.TicketTypeID == nil || *got.TicketTypeID != billing.ID || got.TypeName != "Billing" {
		t.Errorf("after move: type %v %q", got.TicketTypeID, got.TypeName)
	}

	// Closed tickets stay where they are.
	if _, err := s.CloseTicket(ctx, tk.ID, 42, "adam", ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.MoveTicket(ctx, testGuild, tk.ID, general.ID, general.Name); ok {
		t.Error("closed ticket moved")
	}
}
