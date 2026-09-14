package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

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

// categoryNearlyFull is the channel count at which a category gets a warning.
const categoryNearlyFull = discordx.MaxChannelsPerCategory - 5

// unpingable names the support roles of a ticket type that the bot can't
// mention where its tickets open: roles that aren't mentionable, when the
// bot lacks Mention Everyone there.
func (s setup) unpingable(where discord.GuildChannel, t store.TicketType) []string {
	if s.perms(where).Has(discord.PermissionMentionEveryone) {
		return nil
	}
	var names []string
	for _, id := range t.SupportRoleIDs {
		for _, r := range s.roles {
			if r.ID == id && !r.Mentionable {
				names = append(names, r.Name)
			}
		}
	}
	return names
}

// roleList words a list of role names: "the Helpers role", "the Helpers and
// Mods roles".
func roleList(names []string) string {
	if len(names) == 1 {
		return "the " + names[0] + " role"
	}
	return "the " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " roles"
}

// childCount returns how many channels sit in a category.
func (s setup) childCount(categoryID snowflake.ID) int {
	n := 0
	for _, c := range s.channels {
		if p := c.ParentID(); p != nil && *p == categoryID {
			n++
		}
	}
	return n
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
		case missing == "" && len(s.unpingable(parent, t)) > 0:
			// Discord only pings a role that isn't mentionable when the author
			// can mention all roles. That ping is what brings staff into a
			// private thread, so without it they never see the ticket.
			roles := roleList(s.unpingable(parent, t))
			fix := fmt.Sprintf("In Server Settings → Roles, turn on \"Allow anyone to @mention this role\", or give %s the Mention @everyone, @here and All Roles permission.", s.appName)
			if t.Mode == store.ModeThread {
				fail(fmt.Sprintf("Support staff won't be added to its threads: %s can't be mentioned. %s", roles, fix))
			} else {
				out = append(out, problem{Kind: "ticket_type", ID: t.ID,
					Title:  fmt.Sprintf("%s tickets won't ping the team", t.Name),
					Detail: fmt.Sprintf("%s can't be mentioned, so staff aren't notified of new tickets. %s", "T"+roles[1:], fix)})
			}
		case missing == "" && t.KeepsClosedChannels() && s.channel(*t.ClosedParentID) == nil:
			out = append(out, problem{Kind: "ticket_type", ID: t.ID,
				Title:  fmt.Sprintf("Closed %s tickets can't be kept", t.Name),
				Detail: "The category closed tickets are kept in was deleted, so closed channels are deleted straight away instead. Choose another category."})
		case missing == "" && t.KeepsClosedChannels() && s.childCount(*t.ClosedParentID) >= discordx.MaxChannelsPerCategory:
			out = append(out, problem{Kind: "ticket_type", ID: t.ID,
				Title: fmt.Sprintf("Closed %s tickets can't be kept", t.Name),
				Detail: fmt.Sprintf("The %s category is full: Discord allows %d channels per category, so closed tickets can't move there. Keep them for less time, or choose another category.",
					s.channel(*t.ClosedParentID).Name(), discordx.MaxChannelsPerCategory)})
		case missing == "" && t.Mode == store.ModeChannel && parent != nil:
			// Discord caps a category at 50 channels, and busy servers reach
			// it with open tickets alone. Warn before it stops tickets.
			n := s.childCount(parent.ID())
			if n >= discordx.MaxChannelsPerCategory {
				fail(fmt.Sprintf("The %s category is full: Discord allows %d channels per category. Close some tickets, choose another category, or switch to private threads.",
					parent.Name(), discordx.MaxChannelsPerCategory))
			} else if n >= categoryNearlyFull {
				out = append(out, problem{Kind: "ticket_type", ID: t.ID,
					Title: fmt.Sprintf("%s tickets will stop opening soon", t.Name),
					Detail: fmt.Sprintf("The %s category has %d of the %d channels Discord allows. Once it's full, new tickets fail. Close some tickets, choose another category, or switch to private threads.",
						parent.Name(), n, discordx.MaxChannelsPerCategory)})
			}
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
