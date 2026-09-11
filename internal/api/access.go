package api

import (
	"context"
	"slices"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/ticketsbot/internal/auth"
	"github.com/adammcgrogan/ticketsbot/internal/discordx"
)

// memberCacheTTL bounds how long a member's roles are trusted, so removing a
// dashboard role takes effect quickly.
const memberCacheTTL = 30 * time.Second

// canManage reports whether the user has full control of a guild: owners,
// administrators and members with Manage Server.
func canManage(g discord.OAuth2Guild) bool {
	return g.Owner || g.Permissions.Has(discord.PermissionAdministrator) || g.Permissions.Has(discord.PermissionManageGuild)
}

// canAccess decides whether a user may use a guild's dashboard. Managers
// always can; other members need one of the guild's dashboard roles.
// memberRoles costs a Discord request, so it is only called when needed.
func canAccess(g discord.OAuth2Guild, dashboardRoles []snowflake.ID, memberRoles func() ([]snowflake.ID, error)) (allowed, manager bool, err error) {
	if canManage(g) {
		return true, true, nil
	}
	if len(dashboardRoles) == 0 {
		return false, false, nil
	}
	roles, err := memberRoles()
	if err != nil {
		return false, false, err
	}
	return slices.ContainsFunc(roles, func(id snowflake.ID) bool { return slices.Contains(dashboardRoles, id) }), false, nil
}

// memberRoles returns a user's current roles in a guild, or none if they
// aren't a member.
func (s *Server) memberRoles(ctx context.Context, guildID, userID snowflake.ID) ([]snowflake.ID, error) {
	key := "member:" + guildID.String() + ":" + userID.String()
	if v, ok := s.cache.get(key); ok {
		return v.([]snowflake.ID), nil
	}
	var roles []snowflake.ID
	member, err := s.discord.GetMember(guildID, userID, rest.WithCtx(ctx))
	if err == nil {
		roles = member.RoleIDs
	} else if !discordx.IsCode(err, discordx.CodeUnknownMember) {
		return nil, err
	}
	s.cache.set(key, roles, memberCacheTTL)
	return roles, nil
}

type guildAccess struct {
	guild   discord.OAuth2Guild
	manager bool
}

// access looks up a guild in the user's server list and reports whether they
// may use its dashboard.
func (s *Server) access(ctx context.Context, sess *auth.Session, guildID snowflake.ID) (guildAccess, bool, error) {
	guilds, err := s.auth.Guilds(ctx, sess)
	if err != nil {
		return guildAccess{}, false, err
	}
	for _, g := range guilds {
		if g.ID != guildID {
			continue
		}
		var dashboardRoles []snowflake.ID
		if !canManage(g) {
			st, err := s.store.GetGuildSettings(ctx, guildID)
			if err != nil {
				return guildAccess{}, false, err
			}
			dashboardRoles = st.DashboardRoleIDs
		}
		allowed, manager, err := canAccess(g, dashboardRoles, func() ([]snowflake.ID, error) {
			return s.memberRoles(ctx, guildID, sess.User.ID)
		})
		return guildAccess{guild: g, manager: manager}, allowed, err
	}
	return guildAccess{}, false, nil
}
