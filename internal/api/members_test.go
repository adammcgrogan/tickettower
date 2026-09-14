package api

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestUserIDQuery(t *testing.T) {
	for q, want := range map[string]bool{
		"123456789012345678":    true,
		"12345678901234567":     true,
		"adam":                  false,
		"1234":                  false, // names can be numbers too
		"12345678901234567x":    false,
		"123456789012345678901": false,
	} {
		if _, ok := userIDQuery(q); ok != want {
			t.Errorf("userIDQuery(%q) = %v, want %v", q, ok, want)
		}
	}
}

func TestMemberPermissions(t *testing.T) {
	guild, owner := snowflake.ID(1), snowflake.ID(2)
	roles := []discord.Role{
		{ID: guild, Permissions: discord.PermissionViewChannel},
		{ID: 10, Permissions: discord.PermissionAdministrator},
		{ID: 20, Permissions: discord.PermissionManageMessages},
	}
	member := func(id snowflake.ID, roles ...snowflake.ID) discord.Member {
		return discord.Member{User: discord.User{ID: id}, RoleIDs: roles}
	}
	if p := memberPermissions(guild, owner, member(owner), roles); p != discord.PermissionsAll {
		t.Errorf("owner = %v, want everything", p)
	}
	if p := memberPermissions(guild, owner, member(3, 10), roles); p != discord.PermissionsAll {
		t.Errorf("administrator = %v, want everything", p)
	}
	p := memberPermissions(guild, owner, member(3, 20), roles)
	if !p.Has(discord.PermissionManageMessages|discord.PermissionViewChannel) || p.Has(discord.PermissionAdministrator) {
		t.Errorf("member = %v", p)
	}
}
