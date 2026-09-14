package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestBlocks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, testGuild+1)

	if _, err := s.GetBlock(ctx, testGuild, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBlock before blocking = %v, want ErrNotFound", err)
	}
	b := Block{GuildID: testGuild, UserID: 7, UserName: "troll", Reason: "spam", BlockedBy: 1, BlockedByName: "mod"}
	if err := s.BlockMember(ctx, &b); err != nil {
		t.Fatal(err)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
	got, err := s.GetBlock(ctx, testGuild, 7)
	if err != nil || got.Reason != "spam" || got.UserName != "troll" {
		t.Fatalf("GetBlock = %+v, %v", got, err)
	}
	// Blocking again updates the reason instead of failing.
	b.Reason = "alt account"
	if err := s.BlockMember(ctx, &b); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBlocks(ctx, testGuild)
	if err != nil || len(list) != 1 || list[0].Reason != "alt account" {
		t.Fatalf("ListBlocks = %+v, %v", list, err)
	}
	// Other guilds don't see it.
	if _, err := s.GetBlock(ctx, testGuild+1, 7); !errors.Is(err, ErrNotFound) {
		t.Errorf("block leaked to another guild: %v", err)
	}
	if err := s.UnblockMember(ctx, testGuild, 7); err != nil {
		t.Fatal(err)
	}
	if err := s.UnblockMember(ctx, testGuild, 7); !errors.Is(err, ErrNotFound) {
		t.Errorf("second unblock = %v, want ErrNotFound", err)
	}
}

func TestBlockExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)

	past := time.Now().Add(-time.Hour)
	expired := Block{GuildID: testGuild, UserID: 7, UserName: "troll", BlockedBy: 1, BlockedByName: "mod", ExpiresAt: &past}
	if err := s.BlockMember(ctx, &expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBlock(ctx, testGuild, 7); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBlock for expired block = %v, want ErrNotFound", err)
	}
	list, err := s.ListBlocks(ctx, testGuild)
	if err != nil || len(list) != 0 {
		t.Fatalf("ListBlocks with only an expired block = %+v, %v", list, err)
	}

	future := time.Now().Add(time.Hour)
	active := Block{GuildID: testGuild, UserID: 8, UserName: "spammer", BlockedBy: 1, BlockedByName: "mod", ExpiresAt: &future}
	if err := s.BlockMember(ctx, &active); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBlock(ctx, testGuild, 8)
	if err != nil || got.ExpiresAt == nil || !got.ExpiresAt.Equal(future) {
		t.Fatalf("GetBlock for active block = %+v, %v", got, err)
	}

	if err := s.DeleteExpiredBlocks(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListBlocks(ctx, testGuild)
	if err != nil || len(list) != 1 || list[0].UserID != 8 {
		t.Fatalf("ListBlocks after sweep = %+v, %v", list, err)
	}
}

func TestLastClosedAt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	tt := newType(t, s, testGuild, "Support")
	other := newType(t, s, testGuild, "Billing")

	at, err := s.LastClosedAt(ctx, testGuild, tt.ID, 42)
	if err != nil || at != nil {
		t.Fatalf("LastClosedAt with no tickets = %v, %v", at, err)
	}
	open := func(typeID int64, channel snowflake.ID) Ticket {
		n, _ := s.NextTicketNumber(ctx, testGuild)
		tk := Ticket{GuildID: testGuild, Number: n, TicketTypeID: &typeID, TypeName: "x", Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "member"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	first := open(tt.ID, 1)
	if _, err := s.CloseTicket(ctx, first.ID, 1, "mod", ""); err != nil {
		t.Fatal(err)
	}
	// An open ticket and a closed ticket of another type don't count.
	open(tt.ID, 2)
	third := open(other.ID, 3)
	if _, err := s.CloseTicket(ctx, third.ID, 1, "mod", ""); err != nil {
		t.Fatal(err)
	}
	at, err = s.LastClosedAt(ctx, testGuild, tt.ID, 42)
	if err != nil || at == nil || time.Since(*at) > time.Minute {
		t.Fatalf("LastClosedAt = %v, %v", at, err)
	}
	if at, _ := s.LastClosedAt(ctx, testGuild, tt.ID, 43); at != nil {
		t.Errorf("another member's LastClosedAt = %v, want nil", at)
	}
}
