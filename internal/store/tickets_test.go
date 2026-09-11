package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

const testGuild = snowflake.ID(2001)

func seedGuild(t *testing.T, s *Store, id snowflake.ID) {
	t.Helper()
	if err := s.UpsertGuild(context.Background(), Guild{ID: id, Name: "Guild", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
}

func newType(t *testing.T, s *Store, guildID snowflake.ID, name string) TicketType {
	t.Helper()
	parent := snowflake.ID(555)
	tt := TicketType{
		GuildID: guildID, Name: name, Mode: ModeChannel, ParentID: &parent,
		SupportRoleIDs: []snowflake.ID{10, 20}, NameFormat: "ticket-{number}", MaxOpenPerUser: 1,
	}
	if err := s.CreateTicketType(context.Background(), &tt); err != nil {
		t.Fatal(err)
	}
	return tt
}

func TestTicketTypeCRUD(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)

	tt := newType(t, s, testGuild, "Billing")
	got, err := s.GetTicketType(ctx, testGuild, tt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Billing" || got.Mode != ModeChannel || *got.ParentID != 555 || !slices.Equal(got.SupportRoleIDs, []snowflake.ID{10, 20}) {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.Questions == nil || len(got.Questions) != 0 {
		t.Errorf("questions default = %#v, want empty", got.Questions)
	}

	questions := []Question{
		{Label: "Order number", Placeholder: "#1234", Style: QuestionShort, Required: true},
		{Label: "What happened?", Style: QuestionParagraph},
	}
	got.Name, got.Mode, got.ParentID, got.SupportRoleIDs, got.Questions = "Payments", ModeThread, nil, nil, questions
	if err := s.UpdateTicketType(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTicketType(ctx, testGuild, tt.ID)
	if got.Name != "Payments" || got.Mode != ModeThread || got.ParentID != nil || len(got.SupportRoleIDs) != 0 {
		t.Errorf("update mismatch: %+v", got)
	}
	if !slices.Equal(got.Questions, questions) {
		t.Errorf("questions = %+v, want %+v", got.Questions, questions)
	}

	// Clearing the questions stores an empty list, not JSON null.
	got.Questions = nil
	if err := s.UpdateTicketType(ctx, got); err != nil {
		t.Fatal(err)
	}
	var raw string
	s.pool.QueryRow(ctx, `SELECT questions::text FROM ticket_types WHERE id = $1`, tt.ID).Scan(&raw)
	if raw != "[]" {
		t.Errorf("stored questions = %s, want []", raw)
	}

	// Another guild can't see or delete it.
	if _, err := s.GetTicketType(ctx, 9999, tt.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-guild get: %v", err)
	}
	if err := s.DeleteTicketType(ctx, 9999, tt.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-guild delete: %v", err)
	}
	if err := s.DeleteTicketType(ctx, testGuild, tt.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountTicketTypes(ctx, testGuild); n != 0 {
		t.Errorf("count after delete = %d", n)
	}
}

func TestPanelTypesKeepOrderAndIgnoreOtherGuilds(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)

	a := newType(t, s, testGuild, "A")
	b := newType(t, s, testGuild, "B")
	foreign := newType(t, s, 3001, "Foreign")

	p := Panel{GuildID: testGuild, Title: "Help", Color: 1, Style: PanelButtons, TicketTypeIDs: []int64{b.ID, foreign.ID, a.ID}}
	if err := s.CreatePanel(ctx, &p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPanel(ctx, testGuild, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.TicketTypeIDs, []int64{b.ID, a.ID}) {
		t.Errorf("ticket types = %v, want [%d %d]", got.TicketTypeIDs, b.ID, a.ID)
	}

	// Deleting a type removes it from the panel.
	channel, message := snowflake.ID(1), snowflake.ID(2)
	if err := s.SetPanelMessage(ctx, testGuild, p.ID, &channel, &message); err != nil {
		t.Fatal(err)
	}
	withA, _ := s.PublishedPanelsWithType(ctx, testGuild, a.ID)
	if len(withA) != 1 {
		t.Fatalf("published panels with type = %d, want 1", len(withA))
	}
	if err := s.DeleteTicketType(ctx, testGuild, a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetPanel(ctx, testGuild, p.ID)
	if !slices.Equal(got.TicketTypeIDs, []int64{b.ID}) || *got.MessageID != message {
		t.Errorf("after delete: %+v", got)
	}
}

func TestTicketLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")
	opener := snowflake.ID(42)

	var tickets []Ticket
	for i, channel := range []snowflake.ID{7001, 7002} {
		n, err := s.NextTicketNumber(ctx, testGuild)
		if err != nil {
			t.Fatal(err)
		}
		if n != i+1 {
			t.Errorf("ticket number = %d, want %d", n, i+1)
		}
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: opener, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		tickets = append(tickets, tk)
	}

	open, _ := s.OpenTicketChannels(ctx, testGuild, tt.ID, opener)
	if len(open) != 2 {
		t.Fatalf("open tickets = %v", open)
	}

	// Claiming is first-come; only the claimer can unclaim.
	staff, other := snowflake.ID(100), snowflake.ID(101)
	if ok, _ := s.ClaimTicket(ctx, tickets[0].ID, staff, "staff"); !ok {
		t.Error("first claim failed")
	}
	if ok, _ := s.ClaimTicket(ctx, tickets[0].ID, other, "other"); ok {
		t.Error("second claim should fail")
	}
	if ok, _ := s.UnclaimTicket(ctx, tickets[0].ID, other); ok {
		t.Error("non-claimer unclaim should fail")
	}
	got, _ := s.GetTicketByChannel(ctx, 7001)
	if got.ClaimedBy == nil || *got.ClaimedBy != staff || *got.ClaimedByName != "staff" {
		t.Errorf("claimed by = %v", got.ClaimedBy)
	}

	// Closing is idempotent.
	if ok, _ := s.CloseTicket(ctx, tickets[0].ID, staff, "staff", "done"); !ok {
		t.Error("close failed")
	}
	if ok, _ := s.CloseTicket(ctx, tickets[0].ID, staff, "staff", "again"); ok {
		t.Error("second close should report false")
	}
	if ok, _ := s.CloseTicketByChannel(ctx, 7002, "Channel was deleted"); !ok {
		t.Error("close by channel failed")
	}
	got, _ = s.GetTicketByChannel(ctx, 7001)
	if got.Status != StatusClosed || got.CloseReason != "done" || got.ClosedAt == nil || *got.ClosedByName != "staff" {
		t.Errorf("closed ticket = %+v", got)
	}

	stats, err := s.TicketStats(ctx, testGuild)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Open != 0 || stats.OpenedWeek != 2 {
		t.Errorf("stats = %+v", stats)
	}
	closed, _ := s.ListTickets(ctx, testGuild, StatusClosed, 10)
	all, _ := s.ListTickets(ctx, testGuild, "", 10)
	if len(closed) != 2 || len(all) != 2 {
		t.Errorf("list closed=%d all=%d", len(closed), len(all))
	}
}
