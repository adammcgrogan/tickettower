package discordx

import (
	"slices"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// Permissions computes a member's permissions, following Discord's rules:
// the @everyone role and the member's roles give the server-wide base, then
// a channel's overwrites apply in order (@everyone, the member's roles, the
// member). Pass nil overwrites for server-wide permissions. Administrator
// grants everything.
func Permissions(guildID, userID snowflake.ID, memberRoles []snowflake.ID, roles []discord.Role, overwrites discord.PermissionOverwrites) discord.Permissions {
	var perms discord.Permissions
	for _, r := range roles {
		// The @everyone role shares the guild's ID.
		if r.ID == guildID || slices.Contains(memberRoles, r.ID) {
			perms = perms.Add(r.Permissions)
		}
	}
	if perms.Has(discord.PermissionAdministrator) {
		return discord.PermissionsAll
	}

	if o, ok := overwrites.Role(guildID); ok {
		perms = perms.Remove(o.Deny).Add(o.Allow)
	}
	var allow, deny discord.Permissions
	for _, id := range memberRoles {
		if o, ok := overwrites.Role(id); ok {
			allow, deny = allow.Add(o.Allow), deny.Add(o.Deny)
		}
	}
	perms = perms.Remove(deny).Add(allow)
	if o, ok := overwrites.Member(userID); ok {
		perms = perms.Remove(o.Deny).Add(o.Allow)
	}
	return perms
}

// permissionNames are the names Discord shows for the permissions the bot
// relies on, in the order they're listed.
var permissionNames = []struct {
	perm discord.Permissions
	name string
}{
	{discord.PermissionViewChannel, "View Channels"},
	{discord.PermissionManageChannels, "Manage Channels"},
	{discord.PermissionManageRoles, "Manage Roles"},
	{discord.PermissionSendMessages, "Send Messages"},
	{discord.PermissionSendMessagesInThreads, "Send Messages in Threads"},
	{discord.PermissionCreatePrivateThreads, "Create Private Threads"},
	{discord.PermissionManageThreads, "Manage Threads"},
	{discord.PermissionEmbedLinks, "Embed Links"},
	{discord.PermissionAttachFiles, "Attach Files"},
	{discord.PermissionReadMessageHistory, "Read Message History"},
}

// MissingPermissions lists, by name, which of want aren't in have, e.g.
// "Manage Channels and Manage Roles". It returns "" if none are missing.
func MissingPermissions(have, want discord.Permissions) string {
	var names []string
	for _, p := range permissionNames {
		if want.Has(p.perm) && !have.Has(p.perm) {
			names = append(names, p.name)
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}
