package store

import (
	"context"
	"testing"
)

func TestInviteSources(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, src := range []string{"topgg", "site", "topgg", "topgg", "site", "tiktok"} {
		if err := s.CountInviteClick(ctx, src); err != nil {
			t.Fatal(err)
		}
	}
	// An old click falls outside the window.
	if _, err := s.pool.Exec(ctx, `INSERT INTO invite_clicks (day, source, clicks) VALUES (current_date - 40, 'reddit', 9)`); err != nil {
		t.Fatal(err)
	}

	got, err := s.InviteSources(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	want := []InviteSource{{"topgg", 3}, {"site", 2}, {"tiktok", 1}}
	if len(got) != len(want) {
		t.Fatalf("sources = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestActiveGuildCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, 4001)
	seedGuild(t, s, 4002)
	seedGuild(t, s, 4003)
	if err := s.MarkGuildLeft(ctx, 4003); err != nil {
		t.Fatal(err)
	}
	n, err := s.ActiveGuildCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("active guilds = %d, want 2", n)
	}
}
