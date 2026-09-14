package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

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
		SupportRoleIDs: []snowflake.ID{10, 20}, NameFormat: "ticket-{number}", MaxOpenPerUser: 1, AskRating: true,
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
	if got.Answers == nil || len(got.Answers) != 0 {
		t.Errorf("answers default = %#v, want empty", got.Answers)
	}
	if got.AutoAssign {
		t.Error("auto_assign default = true, want false")
	}

	questions := []Question{
		{Label: "Order number", Placeholder: "#1234", Style: QuestionShort, Required: true},
		{Label: "What happened?", Style: QuestionParagraph},
	}
	answers := []Answer{
		{Title: "Refund times", Body: "Refunds take 3-5 business days."},
	}
	got.Name, got.Mode, got.ParentID, got.SupportRoleIDs, got.Questions, got.Answers, got.AutoAssign =
		"Payments", ModeThread, nil, nil, questions, answers, true
	if err := s.UpdateTicketType(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTicketType(ctx, testGuild, tt.ID)
	if got.Name != "Payments" || got.Mode != ModeThread || got.ParentID != nil || len(got.SupportRoleIDs) != 0 {
		t.Errorf("update mismatch: %+v", got)
	}
	if !got.AutoAssign {
		t.Error("auto_assign after update = false, want true")
	}
	if !slices.Equal(got.Questions, questions) {
		t.Errorf("questions = %+v, want %+v", got.Questions, questions)
	}
	if !slices.Equal(got.Answers, answers) {
		t.Errorf("answers = %+v, want %+v", got.Answers, answers)
	}

	// Clearing the questions and answers stores empty lists, not JSON null.
	got.Questions, got.Answers = nil, nil
	if err := s.UpdateTicketType(ctx, got); err != nil {
		t.Fatal(err)
	}
	var raw, rawAnswers string
	s.pool.QueryRow(ctx, `SELECT questions::text FROM ticket_types WHERE id = $1`, tt.ID).Scan(&raw)
	if raw != "[]" {
		t.Errorf("stored questions = %s, want []", raw)
	}
	s.pool.QueryRow(ctx, `SELECT answers::text FROM ticket_types WHERE id = $1`, tt.ID).Scan(&rawAnswers)
	if rawAnswers != "[]" {
		t.Errorf("stored answers = %s, want []", rawAnswers)
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

	// Assigning replaces the claimer and says who had it; releasing works
	// for anyone and says who had it. Neither touches a closed ticket.
	if prev, ok, err := s.AssignTicket(ctx, tickets[0].ID, other, "other"); err != nil || !ok || prev == nil || *prev != staff {
		t.Errorf("assign over a claim: prev=%v ok=%v err=%v", prev, ok, err)
	}
	if got, _ := s.GetTicketByChannel(ctx, 7001); got.ClaimedBy == nil || *got.ClaimedBy != other || *got.ClaimedByName != "other" {
		t.Errorf("after assign: claimed by = %v", got.ClaimedBy)
	}
	if prev, ok, err := s.ReleaseTicket(ctx, tickets[0].ID); err != nil || !ok || prev != other {
		t.Errorf("release: prev=%v ok=%v err=%v", prev, ok, err)
	}
	if _, ok, _ := s.ReleaseTicket(ctx, tickets[0].ID); ok {
		t.Error("releasing an unclaimed ticket should report false")
	}
	if prev, ok, err := s.AssignTicket(ctx, tickets[0].ID, staff, "staff"); err != nil || !ok || prev != nil {
		t.Errorf("assign unclaimed: prev=%v ok=%v err=%v", prev, ok, err)
	}

	// Closing is idempotent.
	if ok, _ := s.CloseTicket(ctx, tickets[0].ID, staff, "staff", "done"); !ok {
		t.Error("close failed")
	}
	if ok, _ := s.CloseTicket(ctx, tickets[0].ID, staff, "staff", "again"); ok {
		t.Error("second close should report false")
	}
	if _, ok, _ := s.AssignTicket(ctx, tickets[0].ID, other, "other"); ok {
		t.Error("assigning a closed ticket should report false")
	}
	if _, ok, _ := s.ReleaseTicket(ctx, tickets[0].ID); ok {
		t.Error("releasing a closed ticket should report false")
	}
	if ok, _ := s.CloseTicketByChannel(ctx, 7002, "Channel was deleted"); !ok {
		t.Error("close by channel failed")
	}
	got, _ = s.GetTicketByChannel(ctx, 7001)
	if got.Status != StatusClosed || got.CloseReason != "done" || got.ClosedAt == nil || *got.ClosedByName != "staff" {
		t.Errorf("closed ticket = %+v", got)
	}

	// Looking a ticket up by ID is scoped to its guild.
	if got, err := s.GetGuildTicket(ctx, testGuild, tickets[1].ID); err != nil || got.ChannelID != 7002 {
		t.Errorf("GetGuildTicket = %+v, %v", got, err)
	}
	if _, err := s.GetGuildTicket(ctx, 9999, tickets[1].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("other guild's ticket: err = %v, want ErrNotFound", err)
	}

	stats, err := s.TicketStats(ctx, testGuild)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Open != 0 || stats.OpenedWeek != 2 {
		t.Errorf("stats = %+v", stats)
	}
	closed, _ := s.ListTickets(ctx, testGuild, TicketQuery{Status: StatusClosed, Limit: 10})
	all, _ := s.ListTickets(ctx, testGuild, TicketQuery{Limit: 10})
	if len(closed) != 2 || len(all) != 2 {
		t.Errorf("list closed=%d all=%d", len(closed), len(all))
	}
}

func TestListTicketsFiltersAndPages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)
	billing := newType(t, s, testGuild, "Billing")
	general := newType(t, s, testGuild, "General")

	channel := snowflake.ID(8000)
	add := func(guildID snowflake.ID, tt TicketType, number int, opener string) Ticket {
		t.Helper()
		channel++
		tk := Ticket{GuildID: guildID, Number: number, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: opener}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	add(testGuild, billing, 1, "alice")
	second := add(testGuild, general, 2, "bob")
	add(testGuild, billing, 3, "carol_x")
	fourth := add(testGuild, general, 4, "100% dave")
	fifth := add(testGuild, billing, 5, "erin")
	foreign := add(3001, newType(t, s, 3001, "Billing"), 1, "alice")
	s.CloseTicket(ctx, second.ID, 100, "staff", "")
	s.ClaimTicket(ctx, fifth.ID, 100, "zed")

	list := func(q TicketQuery) []int {
		t.Helper()
		if q.Limit == 0 {
			q.Limit = 10
		}
		got, err := s.ListTickets(ctx, testGuild, q)
		if err != nil {
			t.Fatal(err)
		}
		numbers := []int{}
		for _, tk := range got {
			numbers = append(numbers, tk.Number)
		}
		return numbers
	}
	id := func(v int64) *int64 { return &v }

	tests := []struct {
		name string
		q    TicketQuery
		want []int
	}{
		{"newest first", TicketQuery{Limit: 2}, []int{5, 4}},
		{"next page", TicketQuery{Limit: 2, Before: id(fourth.ID)}, []int{3, 2}},
		{"last page", TicketQuery{Limit: 2, Before: id(second.ID)}, []int{1}},
		{"cursor from another guild", TicketQuery{Before: id(foreign.ID)}, []int{}},
		{"status", TicketQuery{Status: StatusClosed}, []int{2}},
		{"ticket type", TicketQuery{TypeID: &billing.ID}, []int{5, 3, 1}},
		{"type and page", TicketQuery{TypeID: &billing.ID, Before: id(fifth.ID)}, []int{3, 1}},
		{"opener name", TicketQuery{Search: "ALI"}, []int{1}},
		{"type name", TicketQuery{Search: "gen"}, []int{4, 2}},
		{"claimer name", TicketQuery{Search: "zed"}, []int{5}},
		{"number", TicketQuery{Search: "4"}, []int{4}},
		{"% is literal", TicketQuery{Search: "%"}, []int{4}},
		{"_ is literal", TicketQuery{Search: "_"}, []int{3}},
		{"no match", TicketQuery{Search: "nobody"}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := list(tt.q); !slices.Equal(got, tt.want) {
				t.Errorf("numbers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListTicketsByOpenerAcrossGuilds(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)
	tt1 := newType(t, s, testGuild, "General")
	tt2 := newType(t, s, 3001, "General")

	channel := snowflake.ID(9000)
	add := func(guildID snowflake.ID, tt TicketType, number int, openerID snowflake.ID) Ticket {
		t.Helper()
		channel++
		tk := Ticket{GuildID: guildID, Number: number, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: openerID, OpenerName: "member"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	first := add(testGuild, tt1, 1, 42)
	second := add(3001, tt2, 1, 42)
	add(testGuild, tt1, 2, 43) // someone else's ticket

	got, err := s.ListTicketsByOpener(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	// Newest first.
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Errorf("got tickets %v, %v; want %v, %v", got[0].ID, got[1].ID, second.ID, first.ID)
	}
	for _, tk := range got {
		if tk.OpenerID != 42 {
			t.Errorf("got ticket opened by %v, want 42", tk.OpenerID)
		}
	}
}

func TestDeleteTicketTypeRefusesWithOpenTickets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Help")

	for i, status := range []string{"open", "open", "closed"} {
		tk := Ticket{GuildID: testGuild, Number: i + 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: snowflake.ID(8000 + i), OpenerID: 42, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		if status == "closed" {
			if _, err := s.CloseTicket(ctx, tk.ID, 1, "staff", "done"); err != nil {
				t.Fatal(err)
			}
		}
	}

	var open *ErrOpenTickets
	err := s.DeleteTicketType(ctx, testGuild, tt.ID)
	if !errors.As(err, &open) || open.Count != 2 {
		t.Fatalf("delete with open tickets: %v, want ErrOpenTickets{2}", err)
	}
	if _, err := s.GetTicketType(ctx, testGuild, tt.ID); err != nil {
		t.Fatalf("type should still exist: %v", err)
	}

	// Once every ticket is closed the type can go, and its closed tickets keep their history.
	for _, ch := range []snowflake.ID{8000, 8001} {
		if _, err := s.CloseTicketByChannel(ctx, ch, "done"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteTicketType(ctx, testGuild, tt.ID); err != nil {
		t.Fatalf("delete after closing: %v", err)
	}
	var n int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE guild_id = $1 AND ticket_type_id IS NULL`, int64(testGuild)).Scan(&n)
	if n != 3 {
		t.Errorf("tickets kept after delete = %d, want 3", n)
	}
}

func TestTicketsToCleanUp(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Help")

	open := func(n int) Ticket {
		t.Helper()
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
			ChannelID: snowflake.ID(8100 + n), OpenerID: 42, OpenerName: "adam"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	stillOpen, botClosed, cleaned, handDeleted := open(1), open(2), open(3), open(4)
	for _, tk := range []Ticket{botClosed, cleaned} {
		if _, err := s.CloseTicket(ctx, tk.ID, 1, "staff", "done"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkChannelCleaned(ctx, cleaned.ID); err != nil {
		t.Fatal(err)
	}
	// A channel deleted by hand is already gone, so it needs no cleanup.
	if _, err := s.CloseTicketByChannel(ctx, handDeleted.ChannelID, "Channel was deleted"); err != nil {
		t.Fatal(err)
	}

	// A close that just happened is still in progress.
	if got, _ := s.TicketsToCleanUp(ctx, time.Now().Add(-time.Minute)); len(got) != 0 {
		t.Errorf("fresh closes to clean up = %d, want 0", len(got))
	}
	got, err := s.TicketsToCleanUp(ctx, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != botClosed.ID {
		t.Fatalf("to clean up = %+v, want only ticket %d", got, botClosed.ID)
	}
	_ = stillOpen

	if err := s.MarkChannelCleaned(ctx, botClosed.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.TicketsToCleanUp(ctx, time.Now().Add(time.Minute)); len(got) != 0 {
		t.Errorf("after marking cleaned = %d, want 0", len(got))
	}
}

func TestReopenTicket(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")

	thread := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeThread,
		ChannelID: 7001, OpenerID: 42, OpenerName: "member"}
	channel := Ticket{GuildID: testGuild, Number: 2, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
		ChannelID: 7002, OpenerID: 42, OpenerName: "member"}
	for _, tk := range []*Ticket{&thread, &channel} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()

	// Open tickets can't be reopened.
	if ok, err := s.ReopenTicket(ctx, thread.ID, now); err != nil || ok {
		t.Fatalf("reopen open ticket = %v, %v", ok, err)
	}
	for _, tk := range []Ticket{thread, channel} {
		if _, err := s.CloseTicket(ctx, tk.ID, 100, "staff", "done"); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkChannelCleaned(ctx, tk.ID); err != nil {
			t.Fatal(err)
		}
	}
	// Channel tickets stay closed: their channel is gone.
	if ok, err := s.ReopenTicket(ctx, channel.ID, now); err != nil || ok {
		t.Fatalf("reopen channel ticket = %v, %v", ok, err)
	}
	ok, err := s.ReopenTicket(ctx, thread.ID, now)
	if err != nil || !ok {
		t.Fatalf("reopen thread ticket = %v, %v", ok, err)
	}
	got, err := s.GetTicket(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusOpen || got.ClosedAt != nil || got.ClosedBy != nil || got.CloseReason != "" || !got.WaitingOnStaff {
		t.Errorf("reopened ticket = %+v", got)
	}
	// It closes again like any open ticket, and is no longer owed a cleanup.
	if due, _ := s.TicketsToCleanUp(ctx, now.Add(time.Hour)); len(due) != 0 {
		t.Errorf("reopened ticket listed for cleanup: %+v", due)
	}
	if ok, _ := s.CloseTicket(ctx, thread.ID, 42, "member", ""); !ok {
		t.Error("could not close the reopened ticket")
	}
}

func TestTicketsCarryFeedback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Billing")
	rated := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel, ChannelID: 9001, OpenerID: 42}
	unrated := Ticket{GuildID: testGuild, Number: 2, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel, ChannelID: 9002, OpenerID: 42}
	for _, tk := range []*Ticket{&rated, &unrated} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatal(err)
		}
		s.CloseTicket(ctx, tk.ID, 100, "staff", "")
	}
	if err := s.SetFeedbackRating(ctx, rated.ID, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFeedbackComment(ctx, rated.ID, "Quick and friendly"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetGuildTicket(ctx, testGuild, rated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Feedback == nil || got.Feedback.Rating != 4 || got.Feedback.Comment != "Quick and friendly" || got.Feedback.CreatedAt.IsZero() {
		t.Errorf("feedback = %+v", got.Feedback)
	}
	list, err := s.ListTickets(ctx, testGuild, TicketQuery{Status: StatusClosed, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d tickets", len(list))
	}
	for _, tk := range list {
		if (tk.ID == rated.ID) != (tk.Feedback != nil) {
			t.Errorf("ticket %d feedback = %+v", tk.Number, tk.Feedback)
		}
	}
}

func TestKeptChannelsReopenAndExpire(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Billing")
	kept := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel, ChannelID: 9101, OpenerID: 42}
	deleted := Ticket{GuildID: testGuild, Number: 2, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel, ChannelID: 9102, OpenerID: 42}
	for _, tk := range []*Ticket{&kept, &deleted} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatal(err)
		}
		s.CloseTicket(ctx, tk.ID, 100, "staff", "")
	}
	now := time.Now()

	// A channel ticket whose channel was deleted can't reopen.
	if ok, _ := s.ReopenTicket(ctx, deleted.ID, now); ok {
		t.Error("reopened a channel ticket whose channel is gone")
	}

	// A kept one can, until its channel is deleted.
	if err := s.KeepChannel(ctx, kept.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTicket(ctx, kept.ID)
	if got.ChannelKeptUntil == nil {
		t.Fatal("kept until not set")
	}
	if due, _ := s.KeptChannelsToDelete(ctx, now); len(due) != 0 {
		t.Errorf("%d channels due before their time", len(due))
	}
	due, err := s.KeptChannelsToDelete(ctx, now.Add(2*time.Hour))
	if err != nil || len(due) != 1 || due[0].ID != kept.ID {
		t.Fatalf("due = %v, err %v", due, err)
	}
	if ok, err := s.ReopenTicket(ctx, kept.ID, now); err != nil || !ok {
		t.Fatalf("reopen kept: ok=%v err=%v", ok, err)
	}
	got, _ = s.GetTicket(ctx, kept.ID)
	if got.Status != StatusOpen || got.ChannelKeptUntil != nil {
		t.Errorf("reopened ticket = status %s, kept until %v", got.Status, got.ChannelKeptUntil)
	}

	// Once the sweep (or someone) deletes the channel, it's forgotten.
	s.CloseTicket(ctx, kept.ID, 100, "staff", "")
	s.KeepChannel(ctx, kept.ID, now)
	if err := s.ForgetKeptChannel(ctx, kept.ChannelID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.ReopenTicket(ctx, kept.ID, now); ok {
		t.Error("reopened after the kept channel was deleted")
	}
}
