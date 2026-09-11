package api

import (
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestCanAccess(t *testing.T) {
	member := discord.OAuth2Guild{ID: 1}
	manager := discord.OAuth2Guild{ID: 1, Permissions: discord.PermissionManageGuild}
	owner := discord.OAuth2Guild{ID: 1, Owner: true}
	dashboard := []snowflake.ID{10, 20}

	tests := []struct {
		name                 string
		guild                discord.OAuth2Guild
		dashboardRoles       []snowflake.ID
		memberRoles          []snowflake.ID
		wantAllowed, wantMgr bool
		wantLookup           bool
	}{
		{"manager", manager, dashboard, nil, true, true, false},
		{"owner", owner, nil, nil, true, true, false},
		{"no dashboard roles", member, nil, []snowflake.ID{10}, false, false, false},
		{"has a dashboard role", member, dashboard, []snowflake.ID{5, 20}, true, false, true},
		{"lacks dashboard roles", member, dashboard, []snowflake.ID{5}, false, false, true},
		{"not a member", member, dashboard, nil, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			looked := false
			allowed, mgr, err := canAccess(tt.guild, tt.dashboardRoles, func() ([]snowflake.ID, error) {
				looked = true
				return tt.memberRoles, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if allowed != tt.wantAllowed || mgr != tt.wantMgr || looked != tt.wantLookup {
				t.Errorf("allowed=%v manager=%v lookup=%v, want %v %v %v",
					allowed, mgr, looked, tt.wantAllowed, tt.wantMgr, tt.wantLookup)
			}
		})
	}

	// A failed role lookup denies access and reports the error.
	boom := errors.New("discord down")
	if allowed, _, err := canAccess(member, dashboard, func() ([]snowflake.ID, error) { return nil, boom }); allowed || err != boom {
		t.Errorf("allowed=%v err=%v", allowed, err)
	}
}
