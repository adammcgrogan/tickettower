package store

import (
	"context"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestAvailability(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, testGuild+1)

	if ok, err := s.IsAvailable(ctx, testGuild, 7); err != nil || ok {
		t.Fatalf("IsAvailable before opting in = %v, %v", ok, err)
	}
	if _, _, ok, err := s.NextAssignee(ctx, testGuild); err != nil || ok {
		t.Fatalf("NextAssignee with an empty pool = %v, %v", ok, err)
	}

	if err := s.SetAvailable(ctx, testGuild, 7, "alice", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAvailable(ctx, testGuild, 8, "bob", true); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.IsAvailable(ctx, testGuild, 7); err != nil || !ok {
		t.Fatalf("IsAvailable after opting in = %v, %v", ok, err)
	}
	list, err := s.ListAvailableStaff(ctx, testGuild)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListAvailableStaff = %+v, %v", list, err)
	}
	// Other guilds don't see the pool.
	if list, err := s.ListAvailableStaff(ctx, testGuild+1); err != nil || len(list) != 0 {
		t.Fatalf("pool leaked to another guild: %+v, %v", list, err)
	}

	// Round robin: each assignee comes back once before anyone repeats.
	first, _, ok, err := s.NextAssignee(ctx, testGuild)
	if err != nil || !ok {
		t.Fatalf("NextAssignee #1 = %v, %v", ok, err)
	}
	second, _, ok, err := s.NextAssignee(ctx, testGuild)
	if err != nil || !ok {
		t.Fatalf("NextAssignee #2 = %v, %v", ok, err)
	}
	if first == second {
		t.Fatalf("NextAssignee picked %d twice in a row", first)
	}
	if !((first == 7 && second == 8) || (first == 8 && second == 7)) {
		t.Fatalf("NextAssignee returned unexpected pair %d, %d", first, second)
	}
	third, _, ok, err := s.NextAssignee(ctx, testGuild)
	if err != nil || !ok || third != first {
		t.Fatalf("NextAssignee #3 = %d, %v, %v, want %d", third, ok, err, first)
	}

	// Opting out removes them from the pool.
	if err := s.SetAvailable(ctx, testGuild, 7, "alice", false); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.IsAvailable(ctx, testGuild, 7); err != nil || ok {
		t.Fatalf("IsAvailable after opting out = %v, %v", ok, err)
	}
	for range 3 {
		uid, _, ok, err := s.NextAssignee(ctx, testGuild)
		if err != nil || !ok || uid != snowflake.ID(8) {
			t.Fatalf("NextAssignee after alice opted out = %d, %v, %v, want 8", uid, ok, err)
		}
	}
}
