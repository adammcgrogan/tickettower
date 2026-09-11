package store

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestTranscriptMessages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")

	tk := Ticket{GuildID: testGuild, Number: 1, TicketTypeID: &tt.ID, TypeName: "Support", Mode: ModeChannel,
		ChannelID: 8001, OpenerID: 42, OpenerName: "adam"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	msg := TicketMessage{
		ID: 900, TicketID: tk.ID, AuthorID: 42, AuthorName: "adam", Content: "hello",
		Embeds:      []Embed{{Title: "Hi", Color: 0x7c6cff}},
		Attachments: []Attachment{{Name: "a.png", URL: "https://cdn/a.png", Size: 10, ContentType: "image/png"}},
		CreatedAt:   now,
	}
	if err := s.InsertTicketMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
	// Duplicate inserts are ignored.
	if err := s.InsertTicketMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
	fields := []EmbedField{{Name: "Order number", Value: "#1234"}}
	if err := s.InsertTicketMessage(ctx, TicketMessage{ID: 901, TicketID: tk.ID, AuthorID: 100, AuthorName: "staff", Content: "hey",
		Embeds: []Embed{{Title: "Support · #1", Fields: fields}}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTicketMessage(ctx, 900, "hello (edited)", nil, &now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkTicketMessageDeleted(ctx, 901); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.ListTicketMessages(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "hello (edited)" || msgs[0].EditedAt == nil || len(msgs[0].Attachments) != 1 || len(msgs[0].Embeds) != 0 {
		t.Errorf("first message = %+v", msgs[0])
	}
	if msgs[1].DeletedAt == nil {
		t.Error("second message should be marked deleted")
	}
	if len(msgs[1].Embeds) != 1 || !slices.Equal(msgs[1].Embeds[0].Fields, fields) {
		t.Errorf("embed fields = %+v", msgs[1].Embeds)
	}

	// First response is only recorded once.
	first := now.Add(time.Minute)
	s.SetFirstResponse(ctx, tk.ID, first)
	s.SetFirstResponse(ctx, tk.ID, first.Add(time.Hour))
	got, _ := s.GetTicket(ctx, tk.ID)
	if got.FirstResponseAt == nil || !got.FirstResponseAt.Equal(first) {
		t.Errorf("first response = %v, want %v", got.FirstResponseAt, first)
	}

	refs, err := s.OpenTicketRefs(ctx)
	if err != nil || len(refs) != 1 || refs[0].ChannelID != 8001 || !refs[0].HasFirstResponse {
		t.Errorf("refs = %+v, err = %v", refs, err)
	}

	// Retention: nothing is purged while the ticket is open or retention is unset.
	days := 0
	if err := s.UpdateGuildSettings(ctx, testGuild, GuildSettings{TranscriptRetentionDays: &days}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.PurgeExpiredTranscripts(ctx); n != 0 {
		t.Errorf("purged %d messages from an open ticket", n)
	}
	s.CloseTicket(ctx, tk.ID, snowflake.ID(100), "staff", "")
	s.pool.Exec(ctx, `UPDATE tickets SET closed_at = now() - interval '1 hour' WHERE id = $1`, tk.ID)
	if n, _ := s.PurgeExpiredTranscripts(ctx); n != 2 {
		t.Errorf("purged %d messages, want 2", n)
	}
	st, _ := s.GetGuildSettings(ctx, testGuild)
	if st.TranscriptRetentionDays == nil || *st.TranscriptRetentionDays != 0 {
		t.Errorf("settings = %+v", st)
	}
}
