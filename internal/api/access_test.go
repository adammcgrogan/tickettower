package api

import (
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestAccessLevel(t *testing.T) {
	member := discord.OAuth2Guild{ID: 1}
	manager := discord.OAuth2Guild{ID: 1, Permissions: discord.PermissionManageGuild}
	owner := discord.OAuth2Guild{ID: 1, Owner: true}
	dashboard := []store.DashboardRole{{RoleID: 10, Level: store.LevelSupport}, {RoleID: 20, Level: store.LevelViewer}}

	tests := []struct {
		name           string
		guild          discord.OAuth2Guild
		dashboardRoles []store.DashboardRole
		memberRoles    []snowflake.ID
		want           store.AccessLevel
		wantLookup     bool
	}{
		{"manager", manager, dashboard, nil, store.LevelOwner, false},
		{"owner", owner, nil, nil, store.LevelOwner, false},
		{"no dashboard roles", member, nil, []snowflake.ID{10}, "", false},
		{"has a dashboard role", member, dashboard, []snowflake.ID{5, 20}, store.LevelViewer, true},
		{"best role wins", member, dashboard, []snowflake.ID{20, 10}, store.LevelSupport, true},
		{"lacks dashboard roles", member, dashboard, []snowflake.ID{5}, "", true},
		{"not a member", member, dashboard, nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			looked := false
			got, err := accessLevel(tt.guild, tt.dashboardRoles, func() ([]snowflake.ID, error) {
				looked = true
				return tt.memberRoles, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want || looked != tt.wantLookup {
				t.Errorf("level=%q lookup=%v, want %q %v", got, looked, tt.want, tt.wantLookup)
			}
		})
	}

	// A failed role lookup denies access and reports the error.
	boom := errors.New("discord down")
	if level, err := accessLevel(member, dashboard, func() ([]snowflake.ID, error) { return nil, boom }); level != "" || err != boom {
		t.Errorf("level=%q err=%v", level, err)
	}
}
