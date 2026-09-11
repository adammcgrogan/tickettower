package store

import (
	"context"
	"slices"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestGuildSettings(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Unknown guilds get defaults rather than an error.
	st, err := s.GetGuildSettings(ctx, 9999)
	if err != nil || st.TranscriptRetentionDays != nil || st.DashboardRoleIDs == nil || len(st.DashboardRoleIDs) != 0 {
		t.Fatalf("defaults = %+v, err = %v", st, err)
	}

	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)
	days := 30
	want := GuildSettings{TranscriptRetentionDays: &days, DashboardRoleIDs: []snowflake.ID{10, 20}}
	if err := s.UpdateGuildSettings(ctx, testGuild, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGuildSettings(ctx, testGuild)
	if err != nil {
		t.Fatal(err)
	}
	if *got.TranscriptRetentionDays != 30 || !slices.Equal(got.DashboardRoleIDs, want.DashboardRoleIDs) {
		t.Errorf("round trip = %+v", got)
	}

	// Only guilds with dashboard roles are returned.
	roles, err := s.DashboardRoles(ctx, []snowflake.ID{testGuild, 3001, 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || !slices.Equal(roles[testGuild], want.DashboardRoleIDs) {
		t.Errorf("dashboard roles = %v", roles)
	}

	// Clearing the roles removes the guild from the lookup.
	if err := s.UpdateGuildSettings(ctx, testGuild, GuildSettings{DashboardRoleIDs: nil}); err != nil {
		t.Fatal(err)
	}
	if roles, _ := s.DashboardRoles(ctx, []snowflake.ID{testGuild}); len(roles) != 0 {
		t.Errorf("after clearing = %v", roles)
	}
}
