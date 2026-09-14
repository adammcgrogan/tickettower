package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// testStore connects to TEST_DATABASE_URL, applies migrations and clears
// data. Tests are skipped when it is unset.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `TRUNCATE guilds CASCADE`); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := testStore(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestGuildLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, b, unknown := snowflake.ID(1001), snowflake.ID(1002), snowflake.ID(1003)

	for _, id := range []snowflake.ID{a, b} {
		if err := s.UpsertGuild(ctx, Guild{ID: id, Name: "Guild", OwnerID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	assertActive(t, s, map[snowflake.ID]bool{a: true, b: true, unknown: false})

	// b was removed while the bot was offline.
	if err := s.MarkGuildsLeftExcept(ctx, []snowflake.ID{a}); err != nil {
		t.Fatal(err)
	}
	assertActive(t, s, map[snowflake.ID]bool{a: true, b: false})

	// Re-adding the bot reactivates the guild.
	if err := s.UpsertGuild(ctx, Guild{ID: b, Name: "Renamed", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkGuildLeft(ctx, a); err != nil {
		t.Fatal(err)
	}
	assertActive(t, s, map[snowflake.ID]bool{a: false, b: true})

	var settings int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM guild_settings`).Scan(&settings); err != nil {
		t.Fatal(err)
	}
	if settings != 2 {
		t.Errorf("guild_settings rows = %d, want 2", settings)
	}
}

func assertActive(t *testing.T, s *Store, want map[snowflake.ID]bool) {
	t.Helper()
	ids := make([]snowflake.ID, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	got, err := s.ActiveGuilds(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	for id, active := range want {
		if got[id] != active {
			t.Errorf("guild %d active = %v, want %v", id, got[id], active)
		}
	}
}

// Leaving a server closes its open tickets, and the loops no longer see them.
func TestLeavingClosesOpenTickets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	gone, kept := snowflake.ID(700), snowflake.ID(701)
	seedGuild(t, s, gone)
	seedGuild(t, s, kept)
	tt := newType(t, s, gone, "Support")
	hours := 12
	tt.AutoCloseHours = &hours
	if err := s.UpdateTicketType(ctx, tt); err != nil {
		t.Fatal(err)
	}
	tk := Ticket{GuildID: gone, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
		ChannelID: 7001, OpenerID: 42, OpenerName: "adam"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}
	other := Ticket{GuildID: kept, Number: 1, TypeName: "Support", Mode: ModeChannel, ChannelID: 7002, OpenerID: 42, OpenerName: "adam"}
	if err := s.CreateTicket(ctx, &other); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkGuildLeft(ctx, gone); err != nil {
		t.Fatal(err)
	}

	// Even before the close, the loops skip the left guild.
	if due, _ := s.TicketsToWarn(ctx, tk.LastActivityAt.Add(48*time.Hour)); len(due) != 0 {
		t.Errorf("auto-close still looks at a left guild's tickets: %v", due)
	}

	n, err := s.CloseTicketsOfLeftGuilds(ctx, "The bot was removed")
	if err != nil || n != 1 {
		t.Fatalf("closed %d, %v; want 1", n, err)
	}
	got, _ := s.GetTicket(ctx, tk.ID)
	if got.Status != StatusClosed || got.ClosedBy != nil || got.CloseReason != "The bot was removed" {
		t.Errorf("ticket after leaving: %+v", got)
	}
	if left, _ := s.TicketsToCleanUp(ctx, time.Now().Add(time.Hour)); len(left) != 0 {
		t.Errorf("cleanup sweep would retry a channel the bot can't reach: %v", left)
	}
	if got, _ := s.GetTicket(ctx, other.ID); got.Status != StatusOpen {
		t.Errorf("another server's ticket was closed")
	}
	if refs, _ := s.OpenTicketRefs(ctx); len(refs) != 1 || refs[0].GuildID != kept {
		t.Errorf("open refs = %+v", refs)
	}
}

func TestDeleteGuildData(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	other := snowflake.ID(2002)
	seedGuild(t, s, testGuild)
	seedGuild(t, s, other)
	tt := newType(t, s, testGuild, "Billing")
	ott := newType(t, s, other, "Other")

	tk := Ticket{GuildID: testGuild, TicketTypeID: &tt.ID, ChannelID: 1, Mode: ModeChannel, OpenerID: 42}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}
	var open *ErrOpenTickets
	if err := s.DeleteGuildData(ctx, testGuild); !errors.As(err, &open) || open.Count != 1 {
		t.Fatalf("delete with an open ticket: %v", err)
	}
	if _, err := s.GetTicketType(ctx, testGuild, tt.ID); err != nil {
		t.Fatalf("a refused delete removed data: %v", err)
	}

	if _, err := s.CloseTicket(ctx, tk.ID, 100, "staff", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGuildData(ctx, testGuild); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTicketType(ctx, testGuild, tt.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("ticket type after delete: %v", err)
	}
	if _, err := s.GetTicket(ctx, tk.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("ticket after delete: %v", err)
	}
	if _, err := s.GetGuildSettings(ctx, testGuild); err != nil {
		t.Errorf("settings should be recreated: %v", err)
	}
	assertActive(t, s, map[snowflake.ID]bool{testGuild: true, other: true})
	if _, err := s.GetTicketType(ctx, other, ott.ID); err != nil {
		t.Errorf("another guild's data was touched: %v", err)
	}
}

func TestPurgeLeftGuilds(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	recent, old, active := snowflake.ID(3001), snowflake.ID(3002), snowflake.ID(3003)
	for _, id := range []snowflake.ID{recent, old, active} {
		seedGuild(t, s, id)
	}
	tt := newType(t, s, old, "Gone")
	if err := s.MarkGuildsLeftExcept(ctx, []snowflake.ID{active}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE guilds SET left_at = now() - interval '31 days' WHERE id = $1`, int64(old)); err != nil {
		t.Fatal(err)
	}

	n, err := s.PurgeLeftGuilds(ctx, 30*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purged %d, err %v; want 1", n, err)
	}
	if _, err := s.GetGuild(ctx, old); !errors.Is(err, ErrNotFound) {
		t.Errorf("old guild after purge: %v", err)
	}
	if _, err := s.GetTicketType(ctx, old, tt.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("old guild's type after purge: %v", err)
	}
	for _, id := range []snowflake.ID{recent, active} {
		if _, err := s.GetGuild(ctx, id); err != nil {
			t.Errorf("guild %d after purge: %v", id, err)
		}
	}
}
