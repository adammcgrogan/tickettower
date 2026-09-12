package store

import (
	"context"
	"errors"
	"testing"
)

func TestSavedReplies(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)

	refund := SavedReply{GuildID: testGuild, Name: "Refund policy", Content: "Hi {user}, refunds take 14 days."}
	if err := s.CreateSavedReply(ctx, &refund); err != nil {
		t.Fatal(err)
	}
	if refund.ID == 0 || refund.UpdatedAt.IsZero() {
		t.Fatalf("created = %+v", refund)
	}
	appeal := SavedReply{GuildID: testGuild, Name: "appeals", Content: "Appeal here."}
	if err := s.CreateSavedReply(ctx, &appeal); err != nil {
		t.Fatal(err)
	}

	// Names are unique in a server, ignoring case, but not across servers.
	dupe := SavedReply{GuildID: testGuild, Name: "REFUND POLICY", Content: "x"}
	if err := s.CreateSavedReply(ctx, &dupe); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("duplicate create err = %v, want ErrDuplicateName", err)
	}
	other := SavedReply{GuildID: 3001, Name: "Refund policy", Content: "Elsewhere."}
	if err := s.CreateSavedReply(ctx, &other); err != nil {
		t.Errorf("same name in another server: %v", err)
	}

	// Listed by name, ignoring case, and only for the guild.
	list, err := s.ListSavedReplies(ctx, testGuild)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != appeal.ID || list[1].ID != refund.ID {
		t.Errorf("list = %+v", list)
	}
	if n, err := s.CountSavedReplies(ctx, testGuild); err != nil || n != 2 {
		t.Errorf("count = %d, %v", n, err)
	}

	refund.Name, refund.Content = "Refunds", "Updated."
	if err := s.UpdateSavedReply(ctx, &refund); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSavedReply(ctx, testGuild, refund.ID)
	if err != nil || got.Name != "Refunds" || got.Content != "Updated." {
		t.Errorf("after update = %+v, %v", got, err)
	}
	appeal.Name = "refunds"
	if err := s.UpdateSavedReply(ctx, &appeal); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("rename to a taken name err = %v, want ErrDuplicateName", err)
	}

	// Another guild can't see, change or delete it.
	if _, err := s.GetSavedReply(ctx, 3001, refund.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-guild get err = %v", err)
	}
	stolen := SavedReply{ID: refund.ID, GuildID: 3001, Name: "Mine", Content: "x"}
	if err := s.UpdateSavedReply(ctx, &stolen); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-guild update err = %v", err)
	}
	if err := s.DeleteSavedReply(ctx, 3001, refund.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-guild delete err = %v", err)
	}

	if err := s.DeleteSavedReply(ctx, testGuild, refund.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSavedReply(ctx, testGuild, refund.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete err = %v", err)
	}
}
