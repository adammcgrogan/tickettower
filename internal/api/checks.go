package api

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/panels"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// What the bot needs where tickets open, and in the log channel. These match
// what openTicket, finishClose and logEvent do.
const (
	channelModePerms = discord.PermissionViewChannel | discord.PermissionManageChannels | discord.PermissionManageRoles |
		discord.PermissionSendMessages | discord.PermissionEmbedLinks | discord.PermissionAttachFiles |
		discord.PermissionReadMessageHistory
	threadModePerms = discord.PermissionViewChannel | discord.PermissionCreatePrivateThreads |
		discord.PermissionSendMessagesInThreads | discord.PermissionManageThreads | discord.PermissionEmbedLinks |
		discord.PermissionReadMessageHistory
	logChannelPerms = discord.PermissionViewChannel | discord.PermissionSendMessages | discord.PermissionEmbedLinks
)

// problem is something in a server's setup that stops tickets working, found
// before a member runs into it.
type problem struct {
	// Kind says where to fix it: "ticket_type" and "panel" (with ID), "unlisted"
	// (a ticket type, by ID, that isn't on any published ticket buttons) or
	// "log_channel".
	Kind   string `json:"kind"`
	ID     int64  `json:"id,omitempty"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// setup is everything setupProblems looks at.
type setup struct {
	appName        string
	guildID, botID snowflake.ID
	botRoles       []snowflake.ID
	roles          []discord.Role
	channels       []discord.GuildChannel
	types          []store.TicketType
	panels         []store.Panel
	logChannel     *snowflake.ID
}

func (s setup) channel(id snowflake.ID) discord.GuildChannel {
	for _, c := range s.channels {
		if c.ID() == id {
			return c
		}
	}
	return nil
}

// perms returns the bot's permissions in a channel, or server-wide for nil.
func (s setup) perms(c discord.GuildChannel) discord.Permissions {
	var overwrites discord.PermissionOverwrites
	if c != nil {
		overwrites = c.PermissionOverwrites()
	}
	return discordx.Permissions(s.guildID, s.botID, s.botRoles, s.roles, overwrites)
}

func setupProblems(s setup) []problem {
	out := []problem{}

	for _, t := range s.types {
		title := fmt.Sprintf("%s tickets can't open", t.Name)
		fail := func(detail string) {
			out = append(out, problem{Kind: "ticket_type", ID: t.ID, Title: title, Detail: detail})
		}
		var parent discord.GuildChannel
		if t.ParentID != nil {
			if parent = s.channel(*t.ParentID); parent == nil {
				if t.Mode == store.ModeThread {
					fail("The channel its threads open in was deleted. Choose another one.")
				} else {
					fail("The category its channels open in was deleted. Choose another one, or none.")
				}
				continue
			}
		} else if t.Mode == store.ModeThread {
			fail("Choose the channel its threads open in.")
			continue
		}

		want := channelModePerms
		if t.Mode == store.ModeThread {
			want = threadModePerms
		}
		missing := discordx.MissingPermissions(s.perms(parent), want)
		switch {
		case missing == "":
		case parent == nil:
			fail(fmt.Sprintf("%s is missing %s in this server. Turn them on for its role in Server Settings.", s.appName, missing))
		case parent.Type() == discord.ChannelTypeGuildCategory:
			fail(fmt.Sprintf("%s is missing %s in the %s category. Turn them on for its role in Server Settings, or in the category's permissions.",
				s.appName, missing, parent.Name()))
		default:
			fail(fmt.Sprintf("%s is missing %s in #%s. Turn them on for its role in Server Settings, or in the channel's permissions.",
				s.appName, missing, parent.Name()))
		}
	}

	var published []store.Panel
	for _, p := range s.panels {
		if p.ChannelID == nil || p.MessageID == nil {
			continue
		}
		published = append(published, p)
		switch {
		case s.channel(*p.ChannelID) == nil:
			out = append(out, problem{Kind: "panel", ID: p.ID,
				Title:  fmt.Sprintf("%q was in a deleted channel", p.Title),
				Detail: "Members can't see these ticket buttons any more. Publish them in another channel."})
		case len(panels.Ordered(p, s.types)) == 0:
			out = append(out, problem{Kind: "panel", ID: p.ID,
				Title:  fmt.Sprintf("%q has no ticket types", p.Title),
				Detail: "Members can't open tickets from it. Add a ticket type and save."})
		}
	}
	// A ticket type nobody can choose is usually one that was forgotten.
	if len(published) > 0 {
		for _, t := range s.types {
			if !slices.ContainsFunc(published, func(p store.Panel) bool { return slices.Contains(p.TicketTypeIDs, t.ID) }) {
				out = append(out, problem{Kind: "unlisted", ID: t.ID,
					Title:  fmt.Sprintf("Members can't choose %s", t.Name),
					Detail: "It isn't on any published ticket buttons. Add it to your ticket buttons to start taking these tickets."})
			}
		}
	}

	if s.logChannel != nil {
		if c := s.channel(*s.logChannel); c == nil {
			out = append(out, problem{Kind: "log_channel", Title: "The ticket log channel was deleted",
				Detail: "Choose a new log channel, or turn the log off."})
		} else if missing := discordx.MissingPermissions(s.perms(c), logChannelPerms); missing != "" {
			out = append(out, problem{Kind: "log_channel", Title: "The ticket log can't post",
				Detail: fmt.Sprintf("%s is missing %s in #%s. Turn them on in the channel's permissions.", s.appName, missing, c.Name())})
		}
	}
	return out
}

// getSetupCheck lists problems with the server's setup, such as permissions
// the bot is missing where tickets open.
func (s *Server) getSetupCheck(w http.ResponseWriter, r *http.Request) {
	ctx, g := r.Context(), guildFrom(r)
	st := setup{appName: s.cfg.AppName, guildID: g.ID, botID: s.cfg.DiscordClientID}
	var err error
	if st.channels, err = s.rawChannels(ctx, g.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	if st.roles, err = s.rawRoles(ctx, g.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	// The bot's user ID is its application ID.
	if st.botRoles, err = s.memberRoles(ctx, g.ID, s.cfg.DiscordClientID); err != nil {
		s.writeFailure(w, err)
		return
	}
	if st.types, err = s.store.ListTicketTypes(ctx, g.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	if st.panels, err = s.store.ListPanels(ctx, g.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	settings, err := s.store.GetGuildSettings(ctx, g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	st.logChannel = settings.LogChannelID
	writeJSON(w, http.StatusOK, map[string]any{"problems": setupProblems(st)})
}
