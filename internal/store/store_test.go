package store

import (
	"context"
	"os"
	"testing"

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
