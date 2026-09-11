package discordx

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestPermissions(t *testing.T) {
	const (
		guild = snowflake.ID(1)
		bot   = snowflake.ID(2)
		role  = snowflake.ID(3)
		other = snowflake.ID(4)
	)
	roles := []discord.Role{
		{ID: guild, Permissions: discord.PermissionViewChannel | discord.PermissionSendMessages},
		{ID: role, Permissions: discord.PermissionManageChannels},
		{ID: other, Permissions: discord.PermissionManageRoles},
	}
	view, send, manage := discord.PermissionViewChannel, discord.PermissionSendMessages, discord.PermissionManageChannels

	tests := []struct {
		name       string
		roles      []discord.Role
		overwrites discord.PermissionOverwrites
		want, not  discord.Permissions
	}{
		{name: "server-wide from @everyone and own roles", roles: roles,
			want: view | send | manage, not: discord.PermissionManageRoles},
		{name: "@everyone deny", roles: roles,
			overwrites: discord.PermissionOverwrites{discord.RolePermissionOverwrite{RoleID: guild, Deny: view}},
			want:       send | manage, not: view},
		{name: "role allow beats @everyone deny", roles: roles,
			overwrites: discord.PermissionOverwrites{
				discord.RolePermissionOverwrite{RoleID: guild, Deny: view},
				discord.RolePermissionOverwrite{RoleID: role, Allow: view},
			},
			want: view},
		{name: "own role's deny applies, other roles' allows don't", roles: roles,
			overwrites: discord.PermissionOverwrites{
				discord.RolePermissionOverwrite{RoleID: role, Deny: send},
				discord.RolePermissionOverwrite{RoleID: role + 100, Allow: send},
			},
			not: send},
		{name: "member overwrite wins", roles: roles,
			overwrites: discord.PermissionOverwrites{
				discord.RolePermissionOverwrite{RoleID: role, Allow: send},
				discord.MemberPermissionOverwrite{UserID: bot, Deny: send},
			},
			not: send},
		{name: "roles the member lacks don't count", roles: roles,
			overwrites: discord.PermissionOverwrites{discord.RolePermissionOverwrite{RoleID: other, Deny: view}},
			want:       view},
		{name: "administrator ignores overwrites",
			roles:      []discord.Role{{ID: guild}, {ID: role, Permissions: discord.PermissionAdministrator}},
			overwrites: discord.PermissionOverwrites{discord.MemberPermissionOverwrite{UserID: bot, Deny: view}},
			want:       view | manage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Permissions(guild, bot, []snowflake.ID{role}, tt.roles, tt.overwrites)
			if tt.want != 0 && !got.Has(tt.want) {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if tt.not != 0 && got.Has(tt.not) {
				t.Errorf("got %s, didn't want %s", got, tt.not)
			}
		})
	}
}

func TestMissingPermissions(t *testing.T) {
	have := discord.PermissionViewChannel
	if got := MissingPermissions(have, discord.PermissionViewChannel); got != "" {
		t.Errorf("nothing missing: got %q", got)
	}
	got := MissingPermissions(have, discord.PermissionViewChannel|discord.PermissionManageChannels|
		discord.PermissionManageRoles|discord.PermissionEmbedLinks)
	if want := "Manage Channels, Manage Roles and Embed Links"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
