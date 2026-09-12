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
	if err != nil || st.TranscriptRetentionDays != nil || st.DashboardRoles == nil || len(st.DashboardRoles) != 0 || st.LogChannelID != nil {
		t.Fatalf("defaults = %+v, err = %v", st, err)
	}

	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)
	days, logChannel := 30, snowflake.ID(4242)
	roles := []DashboardRole{{RoleID: 10, Level: LevelAdmin}, {RoleID: 20, Level: LevelViewer}}
	want := GuildSettings{TranscriptRetentionDays: &days, DashboardRoles: roles, LogChannelID: &logChannel}
	if err := s.UpdateGuildSettings(ctx, testGuild, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGuildSettings(ctx, testGuild)
	if err != nil {
		t.Fatal(err)
	}
	if *got.TranscriptRetentionDays != 30 || !slices.Equal(got.DashboardRoles, roles) ||
		got.LogChannelID == nil || *got.LogChannelID != logChannel {
		t.Errorf("round trip = %+v", got)
	}

	// Only guilds with dashboard roles are returned.
	byGuild, err := s.DashboardRoles(ctx, []snowflake.ID{testGuild, 3001, 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(byGuild) != 1 || !slices.Equal(byGuild[testGuild], roles) {
		t.Errorf("dashboard roles = %v", byGuild)
	}

	// Clearing the roles removes the guild from the lookup.
	if err := s.UpdateGuildSettings(ctx, testGuild, GuildSettings{DashboardRoles: nil}); err != nil {
		t.Fatal(err)
	}
	if byGuild, _ := s.DashboardRoles(ctx, []snowflake.ID{testGuild}); len(byGuild) != 0 {
		t.Errorf("after clearing = %v", byGuild)
	}
	if got, _ := s.GetGuildSettings(ctx, testGuild); got.LogChannelID != nil || got.TranscriptRetentionDays != nil {
		t.Errorf("after clearing = %+v", got)
	}
}

func TestHighestLevel(t *testing.T) {
	roles := []DashboardRole{{RoleID: 1, Level: LevelViewer}, {RoleID: 2, Level: LevelAdmin}, {RoleID: 3, Level: LevelSupport}}
	cases := []struct {
		member []snowflake.ID
		want   AccessLevel
	}{
		{nil, ""},
		{[]snowflake.ID{9}, ""},
		{[]snowflake.ID{1}, LevelViewer},
		{[]snowflake.ID{1, 3}, LevelSupport},
		{[]snowflake.ID{3, 2, 1}, LevelAdmin},
	}
	for _, c := range cases {
		if got := HighestLevel(roles, c.member); got != c.want {
			t.Errorf("HighestLevel(%v) = %q, want %q", c.member, got, c.want)
		}
	}
	if !LevelOwner.AtLeast(LevelAdmin) || LevelSupport.AtLeast(LevelAdmin) || !LevelSupport.AtLeast(LevelViewer) {
		t.Error("AtLeast ordering is wrong")
	}
}
