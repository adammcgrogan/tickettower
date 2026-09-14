package store

import (
	"context"
	"errors"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestTicketNotes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)

	tk := Ticket{GuildID: testGuild, Number: 1, TypeName: "Billing", Mode: ModeChannel, ChannelID: 8101, OpenerID: 1, OpenerName: "member"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}
	first := TicketNote{TicketID: tk.ID, AuthorID: 100, AuthorName: "Sam", Content: "Refunded $20, don't refund again."}
	if err := s.AddTicketNote(ctx, testGuild, &first); err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.CreatedAt.IsZero() {
		t.Fatalf("added = %+v", first)
	}
	second := TicketNote{TicketID: tk.ID, AuthorID: 101, AuthorName: "Alex", Content: "This is an alt of someone we banned."}
	if err := s.AddTicketNote(ctx, testGuild, &second); err != nil {
		t.Fatal(err)
	}

	// Another server can neither add notes to the ticket nor read them.
	other := TicketNote{TicketID: tk.ID, AuthorID: 102, AuthorName: "Mallory", Content: "x"}
	if err := s.AddTicketNote(ctx, 3001, &other); !errors.Is(err, ErrNotFound) {
		t.Errorf("note from another server: err = %v, want ErrNotFound", err)
	}
	if notes, err := s.ListTicketNotes(ctx, 3001, tk.ID); err != nil || len(notes) != 0 {
		t.Errorf("another server's list = %+v, %v", notes, err)
	}

	notes, err := s.ListTicketNotes(ctx, testGuild, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].ID != first.ID || notes[1].ID != second.ID ||
		notes[0].AuthorID != 100 || notes[0].AuthorName != "Sam" || notes[0].Content != first.Content {
		t.Errorf("notes = %+v", notes)
	}

	// Notes go with the transcript when retention removes it.
	days := 0
	if err := s.UpdateGuildSettings(ctx, testGuild, GuildSettings{TranscriptRetentionDays: &days}); err != nil {
		t.Fatal(err)
	}
	s.CloseTicket(ctx, tk.ID, snowflake.ID(100), "Sam", "")
	s.pool.Exec(ctx, `UPDATE tickets SET closed_at = now() - interval '1 hour' WHERE id = $1`, tk.ID)
	if n, err := s.PurgeExpiredTranscripts(ctx); err != nil || n != 2 {
		t.Errorf("purged %d rows, %v; want the 2 notes", n, err)
	}
	if notes, _ := s.ListTicketNotes(ctx, testGuild, tk.ID); len(notes) != 0 {
		t.Errorf("notes after the purge = %+v", notes)
	}
}
